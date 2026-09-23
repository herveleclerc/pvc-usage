# Disques fantômes et factures astronomiques : dompter la condition Unused de Kubernetes 1.37 avec un plugin kubectl en Go

Par Hervé Leclerc - Septembre 2026

Il est 17h45 un vendredi de clôture comptable. Votre responsable FinOps déboule avec un graphique dont la pente ferait pâlir une étape alpestre du Tour de France : la facture de stockage cloud du cluster de pré-production a triplé en deux mois. Pourtant, aucun déploiement applicatif majeur n'a eu lieu. Les pods ont été arrêtés, les tests sont finis, les namespaces sont presque déserts. Mais dans l'ombre du plan de contrôle, une légion silencieuse continue de prélever son tribut : des centaines de PersistentVolumeClaims (PVC) orphelines, solidement arrimées à des disques réseau facturés au gigaoctet par heure.

Kubernetes a été conçu pour choyer vos données. Lorsqu'un pod s'éteint, son stockage persiste. C'est rassurant pour une base de données de production ; c'est un gouffre financier lorsqu'un pipeline de CI génère un volume de 50 Go par tir de test éphémère.

Jusqu'à présent, savoir si une PVC était réellement utilisée par une charge de travail active relevait de l'archéologie scriptée à base de `jq` et d'injonctions mystiques. Avec la version 1.37 de Kubernetes, la donne change grâce à la promotion en Beta du KEP-5541 : `PersistentVolumeClaimUnusedSinceTime`.

Dans cet article, nous allons disséquer cette nouvelle fonctionnalité, comprendre la mécanique fine du contrôleur de protection des PVC, puis coder pas à pas un plugin `kubectl` officiel en Go, multi-architecture, compilé pour Linux, macOS et Windows, prêt pour distribution via Krew.

Garantie formelle : pas de fioritures, pas de raccourcis, du vrai code Go utilisant `cli-runtime` et zéro ligne de script bash bancal.

---

## 1. La galère historique : traquer l'inactivité d'une PVC

Avant Kubernetes 1.37, déterminer si un volume persistant était rattaché à un pod actif obligeait les administrateurs à croiser plusieurs flux d'API :

1. Lister tous les pods du namespace ou du cluster.
2. Parcourir chaque pod pour inspecter `.spec.volumes[*].persistentVolumeClaim.claimName`.
3. Vérifier l'état d'exécution du pod (car un pod en phase `Succeeded` ou `Completed` conserve la référence au volume dans sa spécification, sans pour autant monter le disque !).
4. Déduire par soustraction les PVC orphelines.

Certains courageux rédigeaient des requêtes d'anthologie dans leur terminal :

```bash
kubectl get pvc -A -o json | jq -r '
  .items[] 
  | select(.status.conditions[]? | select(.type=="Unused" and .status=="True")) 
  | select( (.status.conditions[] | select(.type=="Unused") | .lastTransitionTime) as $t 
    | (now - ($t | fromdateiso8601)) > (30 * 86400) ) 
  | "\(.metadata.namespace)/\(.metadata.name) inutilisé depuis \(.status.conditions[] | select(.type=="Unused") | .lastTransitionTime)"'
```

Outre l'illisibilité de la commande, ce genre de bricolage souffre d'un défaut rédhibitoire : il est ponctuel. Il ne capture pas l'historique d'inactivité. Si aucun pod ne tourne au moment précis où le script s'exécute, la PVC est-elle orpheline depuis trois minutes (redémarrage d'un StatefulSet) ou depuis six mois (test oublié) ? Impossible de le savoir sans corrélation d'événements externes ou métriques Prometheus conservées au prix fort.

---

## 2. Kubernetes 1.37 et le KEP-5541 : anatomie de la condition Unused

Kubernetes v1.36 avait introduit une lueur d'espoir sous feature gate Alpha. La version v1.37 promeut la fonctionnalité `PersistentVolumeClaimUnusedSinceTime` en Beta, activée par défaut.

### Qui fait le travail ?
Le composant responsable de cette mise à jour n'est pas un nouvel agent obscur, mais un contrôleur éprouvé du `kube-controller-manager` : le **PVC Protection Controller**.

Ce contrôleur surveillait déjà en continu les pods et les PVCs pour appliquer le finalizer `kubernetes.io/pvc-protection`, empêchant la suppression accidentelle d'une PVC pendant qu'un pod l'exploite. Puisqu'il maintient déjà un cache indexé des pods référençant chaque PVC, le SIG Storage lui a logiquement confié l'évaluation de la condition d'usage.

### La nouvelle condition dans l'API

Désormais, le champ `.status.conditions` d'une `PersistentVolumeClaim` accueille une condition de type `Unused` :

```yaml
status:
  accessModes:
  - ReadWriteOnce
  capacity:
    storage: 50Gi
  conditions:
  - lastProbeTime: null
    lastTransitionTime: "2026-09-14T10:15:32Z"
    message: No pods are currently referencing this PVC
    reason: NoPodsUsingPVC
    status: "True"
    type: Unused
  phase: Bound
```

### Règles sémantiques et cas particuliers

Le contrôleur applique une logique stricte :

1. **Les pods terminés ne comptent pas** :
   Un pod en phase `Succeeded` ou `Failed` (comme un Job batch avec `restartPolicy: Never`) ne bloque pas la transition vers `Unused=True`. Dès la terminaison du conteneur, le volume est considéré comme libéré.
2. **Les pods en attente (Pending) comptent** :
   Même si un pod ne peut pas être planifié immédiatement (faute de ressources ou à cause d'un node selector trop restrictif), son intention d'utiliser le volume est prioritaire. La PVC reste marquée `Unused=False` avec la raison `PodUsingPVC`.
3. **Multi-consommateurs** :
   Pour les volumes partagés (`ReadWriteMany` ou montages multiples), la condition bascule à `Unused=True` uniquement après la terminaison du tout dernier pod consommateur.
4. **Le trésor caché : `lastTransitionTime`** :
   Chaque condition standard Kubernetes enregistre l'horodatage exact de sa dernière transition d'état. Lorsque le statut passe de `False` à `True`, `lastTransitionTime` fige la seconde exacte où le volume est devenu orphelin.

---

## 3. Pourquoi concevoir un plugin kubectl en Go ?

Lire des conditions JSON brutes ou fabriquer des scripts bash artisanaux n'est pas viable en environnement d'entreprise. Pour intégrer cette fonctionnalité dans le quotidien des équipes de développement et des opérations, la création d'un binaire respectant l'écosystème CLI de Kubernetes s'impose.

### Les règles de l'art pour un plugin kubectl

Pour qu'un binaire Go devienne un véritable citoyen de première classe dans `kubectl`, plusieurs principes architecturaux doivent être respectés :

1. **Convention de nommage et PATH** :
   Tout exécutable nommé `kubectl-<nom>` disponible dans la variable d'environnement `$PATH` de l'utilisateur est automatiquement reconnu par `kubectl`. Taper `kubectl pvc-usage` déclenche l'exécution de `kubectl-pvc-usage`.
2. **Ne jamais réinventer la gestion du Kubeconfig** :
   Un plugin amateur parse les arguments à la main ou appelle `clientcmd.BuildConfigFromFlags("", os.Getenv("KUBECONFIG"))`. C'est une erreur. En utilisant la bibliothèque officielle `k8s.io/cli-runtime/pkg/genericclioptions`, le plugin hérite automatiquement de tous les drapeaux natifs de `kubectl` :
   - `--kubeconfig`
   - `--context`
   - `--namespace` / `-n`
   - `--cluster`
   - `--user`
   - `--as` (impersonation)
   - `--token`
   - `--insecure-skip-tls-verify`
3. **Utiliser Cobra pour l'arbre des commandes** :
   Le framework `github.com/spf13/cobra` fournit la structure standard pour les sous-commandes, l'aide contextuelle, l'autocomplétion et la gestion des drapeaux via `pflag`.

---

## 4. Architecture et implémentation de `kubectl-pvc-usage`

Le projet est structuré selon le layout idiomatique Go :

```text
pvc-usage/
├── cmd/
│   └── kubectl-pvc-usage/
│       └── main.go
├── pkg/
│   ├── cmd/
│   │   ├── root.go
│   │   ├── root_test.go
│   │   └── version.go
│   ├── collector/
│   │   ├── collector.go
│   │   ├── collector_test.go
│   │   ├── duration.go
│   │   └── duration_test.go
│   ├── printer/
│   │   ├── printer.go
│   │   └── printer_test.go
│   ├── types/
│   │   └── types.go
│   └── version/
│       └── version.go
├── Makefile
├── .goreleaser.yaml
└── krew.yaml
```

Examinons les pièces maîtresses du code.

### 4.1. L'intégration de cli-runtime et Cobra (`pkg/cmd/root.go`)

Voici comment coupler les drapeaux Kubernetes officiels avec nos options personnalisées :

```go
package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"

	"github.com/herveleclerc/pvc-usage/pkg/collector"
	"github.com/herveleclerc/pvc-usage/pkg/printer"
	"github.com/herveleclerc/pvc-usage/pkg/types"
)

type RootOptions struct {
	configFlags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams

	allNamespaces bool
	unusedOnly    bool
	inUseOnly     bool
	minAgeStr     string
	storageClass  string
	sortBy        string
	showPods      bool
	output        string
	showSummary   bool
	noHeaders     bool
}

func NewCmdRoot(streams genericclioptions.IOStreams) *cobra.Command {
	o := &RootOptions{
		configFlags: genericclioptions.NewConfigFlags(true),
		IOStreams:   streams,
		showSummary: true,
		sortBy:      "namespace",
		output:      "table",
	}

	cmd := &cobra.Command{
		Use:   "pvc-usage",
		Short: "Inspect PersistentVolumeClaim usage and idle duration using Kubernetes 1.37 Unused condition",
		RunE: func(cmd *cobra.Command, args []string) error {
			return o.Run(cmd.Context())
		},
	}

	// Injection des drapeaux officiels de kubectl
	o.configFlags.AddFlags(cmd.Flags())

	// Drapeaux applicatifs specifiques au plugin
	cmd.Flags().BoolVarP(&o.allNamespaces, "all-namespaces", "A", false, "Lister sur tous les namespaces")
	cmd.Flags().BoolVarP(&o.unusedOnly, "unused-only", "u", false, "Afficher uniquement les PVCs inutilisees")
	cmd.Flags().BoolVar(&o.inUseOnly, "in-use-only", false, "Afficher uniquement les PVCs en cours d'utilisation")
	cmd.Flags().StringVar(&o.minAgeStr, "min-age", "", "Filtrer les PVCs inutilisees depuis au moins cette duree (ex: '30d', '7d', '24h')")
	cmd.Flags().StringVar(&o.storageClass, "storage-class", "", "Filtrer par StorageClass")
	cmd.Flags().StringVar(&o.sortBy, "sort-by", "namespace", "Tri des resultats: 'namespace', 'name', 'unused', 'size', 'age'")
	cmd.Flags().BoolVar(&o.showPods, "show-pods", false, "Inspecter les pods connectes (mode wide)")
	cmd.Flags().StringVarP(&o.output, "output", "o", "table", "Format de sortie: 'table', 'wide', 'json', 'yaml'")
	cmd.Flags().BoolVar(&o.showSummary, "summary", true, "Afficher le bilan FinOps en fin de tableau")

	return cmd
}
```

En une ligne (`o.configFlags.AddFlags(cmd.Flags())`), l'utilisateur bénéficie d'une ergonomie rigoureusement identique à celle de `kubectl get` ou `kubectl describe`.

### 4.2. L'évaluation de la condition Unused (`pkg/collector/collector.go`)

Le collecteur inspecte le tableau `Conditions` du statut de la PVC. Si le cluster tourne sur une version antérieure à 1.37 (ou sans le feature gate actif), le statut est dégradé gracieusement :

```go
func (c *Collector) evaluatePVC(pvc corev1.PersistentVolumeClaim, now time.Time) types.PVCUsageInfo {
	info := types.PVCUsageInfo{
		Namespace:         pvc.Namespace,
		Name:              pvc.Name,
		Phase:             pvc.Status.Phase,
		CreationTimestamp: pvc.CreationTimestamp,
		VolumeName:        pvc.Spec.VolumeName,
		AgeHuman:          FormatDuration(now.Sub(pvc.CreationTimestamp.Time)),
	}

	// Extraction de la condition Unused (Kubernetes 1.37 / KEP-5541)
	var unusedCondition *corev1.PersistentVolumeClaimCondition
	for i := range pvc.Status.Conditions {
		if pvc.Status.Conditions[i].Type == "Unused" {
			unusedCondition = &pvc.Status.Conditions[i]
			break
		}
	}

	if unusedCondition == nil {
		info.HasUnusedCondition = false
		info.UsageStatus = types.StatusMissing
		info.IsUnused = false
		info.UnusedDurationStr = "N/A (K8s < 1.37)"
		return info
	}

	info.HasUnusedCondition = true
	info.Reason = unusedCondition.Reason
	info.Message = unusedCondition.Message

	switch unusedCondition.Status {
	case corev1.ConditionTrue:
		info.IsUnused = true
		info.UsageStatus = types.StatusUnused
		info.UnusedSince = &unusedCondition.LastTransitionTime
		duration := now.Sub(unusedCondition.LastTransitionTime.Time)
		if duration < 0 {
			duration = 0
		}
		info.UnusedDuration = duration
		info.UnusedDurationStr = FormatDuration(duration)

	case corev1.ConditionFalse:
		info.IsUnused = false
		info.UsageStatus = types.StatusInUse
		info.UnusedDurationStr = "Active"

	default:
		info.IsUnused = false
		info.UsageStatus = types.StatusUnknown
		info.UnusedDurationStr = string(unusedCondition.Status)
	}

	return info
}
```

### 4.3. Gestion des durées étendues avec les jours (`30d`, `7d`)

La fonction standard `time.ParseDuration` de Go s'arrête aux heures (`h`). Or, en matière de stockage dormant, l'unité de référence est le jour ou la semaine. Nous avons donc implémenté un parser étendu acceptant la syntaxe `d` :

```go
var dayRegex = regexp.MustCompile(`^(\d+)([dD])$`)

func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	matches := dayRegex.FindStringSubmatch(s)
	if len(matches) == 3 {
		days, err := strconv.Atoi(matches[1])
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}

	return time.ParseDuration(s)
}
```

### 4.4. Affichage tabulaire et bilan FinOps

Un outil en ligne de commande doit être synthétique. `kubectl-pvc-usage` utilise `text/tabwriter` pour un alignement parfait des colonnes et conclut par un récapitulatif du volume de données orphelines :

```text
NAMESPACE       NAME       STATUS  UNUSED-SINCE  CAPACITY  STORAGECLASS  AGE
production      pg-backup  Unused  42d 8h        200Gi     gp3-encrypted 90d
staging         redis-tmp  Unused  15d 2h        20Gi      standard      30d
test-env        es-data    InUse   Active        100Gi     fast-nvme     12d

--- Storage Usage Summary (FinOps) ---
Total PVCs examined:   3 (Total capacity: 320Gi)
Unused PVCs:           2 (Orphaned capacity: 220Gi)
Active in-use PVCs:    1
```

---

## 5. Industrialisation : compilation multi-architecture et Krew

Un plugin kubectl écrit en Go ne doit pas forcer l'utilisateur à installer un compilateur local. Le projet intègre deux méthodes de distribution complémentaires.

### Le Makefile universel

Le fichier `Makefile` pilote la compilation locale et croisée en exploitant les capacités natives du compilateur Go (`CGO_ENABLED=0`) :

```makefile
PLATFORMS := \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64

build-all:
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		OS=$${platform%/*}; \
		ARCH=$${platform#*/}; \
		OUTPUT=dist/$(BINARY_NAME)-$${OS}-$${ARCH}; \
		if [ "$${OS}" = "windows" ]; then OUTPUT="$${OUTPUT}.exe"; fi; \
		echo "Building for $${OS}/$${ARCH}..."; \
		GOOS=$${OS} GOARCH=$${ARCH} CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o "$${OUTPUT}" ./cmd/kubectl-pvc-usage || exit 1; \
	done
```

### GoReleaser et packaging Krew

Pour les releases GitHub officielles, `.goreleaser.yaml` automatise la génération des archives tarball et du fichier de contrôle des empreintes cryptographiques (`checksums.txt`).

Le fichier de métadonnées `krew.yaml` permet de soumettre directement le plugin à l'index communautaire de Krew :

```yaml
apiVersion: krew.googlecontainertools.github.com/v1alpha2
kind: Plugin
metadata:
  name: pvc-usage
spec:
  version: "v0.1.0"
  homepage: https://github.com/herveleclerc/pvc-usage
  shortDescription: Identify unused PVCs and reclaim orphaned storage in Kubernetes 1.37+
  description: |
    kubectl-pvc-usage detects unused PersistentVolumeClaims natively leveraging
    the Kubernetes 1.37 Beta feature PersistentVolumeClaimUnusedSinceTime (KEP-5541).
  platforms:
  - selector:
      matchExpressions:
      - key: os
        operator: In
        values: [darwin, linux, windows]
    uri: https://github.com/herveleclerc/pvc-usage/releases/download/v0.1.0/kubectl-pvc-usage_{{ .OS }}_{{ .Arch }}.tar.gz
    sha256: "..."
    bin: kubectl-pvc-usage
```

---

## 6. Démonstration pratique sur un cluster Kubernetes 1.37

Voyons le résultat en conditions réelles sur un cluster Kubernetes 1.37.0.

### Étape 1 : Création d'une PVC de test

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: test-data
  namespace: demo
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
```

### Étape 2 : Lancement d'un pod consommateur

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: test-app
  namespace: demo
spec:
  containers:
  - name: app
    image: busybox:1.36
    command: ["sleep", "3600"]
    volumeMounts:
    - name: data
      mountPath: /data
  volumes:
  - name: data
    persistentVolumeClaim:
      claimName: test-data
```

En interrogeant notre plugin avec les options détaillées :

```bash
$ kubectl pvc-usage -n demo -o wide --show-pods
NAMESPACE  NAME       STATUS  UNUSED-SINCE  CAPACITY  STORAGECLASS  VOLUME                                    ACCESSMODES    PODS      AGE  REASON
demo       test-data  InUse   Active        1Gi       standard      pvc-7bd3964b-3e0a-4478-9a00-8e460027273b  ReadWriteOnce  test-app  45s  PodUsingPVC

--- Storage Usage Summary (FinOps) ---
Total PVCs examined:   1 (Total capacity: 1Gi)
Unused PVCs:           0 (Orphaned capacity: 0)
Active in-use PVCs:    1
```

Le plugin identifie immédiatement que le pod `test-app` utilise activement la ressource.

### Étape 3 : Suppression du pod et bascule immédiate

Supprimons le pod consommateur :

```bash
$ kubectl delete pod -n demo test-app
```

Le `pvc-protection-controller` met à jour la PVC. Réinterrogeons le plugin :

```bash
$ kubectl pvc-usage -n demo
NAMESPACE  NAME       STATUS  UNUSED-SINCE  CAPACITY  STORAGECLASS  AGE
demo       test-data  Unused  14s           1Gi       standard      4m

--- Storage Usage Summary (FinOps) ---
Total PVCs examined:   1 (Total capacity: 1Gi)
Unused PVCs:           1 (Orphaned capacity: 1Gi)
Active in-use PVCs:    0
```

Et pour cibler uniquement les volumes dormants oubliés depuis plus d'un mois à l'échelle du cluster :

```bash
$ kubectl pvc-usage -A --unused-only --min-age=30d --sort-by=unused
```

Fini les pipelines de détection fragiles, fini les regex sur des logs de contrôleur CSI.

---

## 7. Ce qu'il faut retenir

La promotion en Beta de la condition `Unused` dans Kubernetes 1.37 comble un manque historique de l'API de stockage :
- L'information d'usage est désormais **déclarative**, intégrée nativement dans `.status.conditions`.
- L'horodatage `lastTransitionTime` fournit une valeur temporelle exploitable pour automatiser des politiques de nettoyage ou de décommissionnement progressif.
- Concevoir un plugin `kubectl` en Go avec `cli-runtime` et `cobra` prend moins d'une journée et offre une expérience utilisateur conforme aux standards du projet Kubernetes.

Le code source complet, les tests unitaires et les workflows de compilation sont disponibles sur GitHub :
[https://github.com/herveleclerc/pvc-usage](https://github.com/herveleclerc/pvc-usage)

Vous n'avez désormais plus aucune excuse pour laisser les disques orphelins grever votre budget cloud.
