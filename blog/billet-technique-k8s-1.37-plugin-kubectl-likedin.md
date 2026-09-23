# Traquer les PVC orphelins : exploiter la condition Unused de Kubernetes 1.37 avec un plugin kubectl en Go

Par Hervé Leclerc - Septembre 2026

Sur Kubernetes, la gestion du stockage réseau génère rapidement des surcoûts invisibles. Lorsqu'un pod est supprimé ou qu'un namespace est nettoyé, les **PersistentVolumeClaims** (PVC) restent conservés par défaut. Ce comportement garantit la durabilité des données, mais il entraîne une accumulation de volumes inutilisés, notamment lors de tirs de CI/CD ou sur des environnements d'intégration éphémères.

Jusqu'à présent, vérifier si un PVC était réellement rattaché à une charge de travail active nécessitait de concevoir des scripts complexes croisant l'API des pods et des volumes. La version 1.37 de Kubernetes simplifie ce suivi en faisant passer en Beta le KEP-5541 : **PersistentVolumeClaimUnusedSinceTime**.

Cet article détaille le fonctionnement de cette nouvelle condition dans l'API Server, le rôle du contrôleur de stockage associé, puis la création pas à pas d'un plugin **kubectl** officiel en Go, distribué via Krew.

---

## 1. Limites des méthodes historiques de détection

Avant Kubernetes 1.37, déterminer l'état d'activité d'un volume persistant imposait de requêter plusieurs ressources du cluster :

1. Lister l'ensemble des pods.
2. Inspecter les déclarations de volumes dans **.spec.volumes[*].persistentVolumeClaim.claimName**.
3. Filtrer la phase d'exécution des pods (les pods en état **Succeeded** ou **Completed** conservent leur référence au volume sans monter le disque).
4. Déduire par soustraction les PVC non rattachés.

Exemple de commande utilisée pour automatiser ce contrôle :

```bash
kubectl get pvc -A -o json | jq -r '
  .items[] 
  | select(.status.conditions[]? | select(.type=="Unused" and .status=="True")) 
  | select( (.status.conditions[] | select(.type=="Unused") | .lastTransitionTime) as $t 
    | (now - ($t | fromdateiso8601)) > (30 * 86400) ) 
  | "\(.metadata.namespace)/\(.metadata.name) inutilisé depuis \(.status.conditions[] | select(.type=="Unused") | .lastTransitionTime)"'

```

Cette approche souffre d'une limite majeure : l'absence d'historique d'inactivité. Si aucun pod ne tourne au moment de l'exécution du script, il est impossible de savoir si le PVC est orphelin depuis quelques minutes (redémarrage applicatif) ou depuis plusieurs mois.

---

## 2. Fonctionnement du KEP-5541 et de la condition Unused

Introduite en Alpha en version 1.36, la fonctionnalité **PersistentVolumeClaimUnusedSinceTime** passe en Beta et devient activée par défaut avec Kubernetes 1.37.

### Rôle du PVC Protection Controller

La mise à jour de cet état est prise en charge dans le **kube-controller-manager** par le **PVC Protection Controller**.

Ce contrôleur surveillait déjà la relation entre pods et PVC pour positionner le finalizer **kubernetes.io/pvc-protection** et éviter la suppression d'un volume en cours d'utilisation. Maintenant déjà un cache indexé des pods associés à chaque PVC, il a été étendu pour calculer la condition d'usage.

### Structure de la condition dans l'API

Le champ **.status.conditions** d'un PVC intègre désormais le type **Unused** :

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

### Règles de gestion d'état

1. **Pods terminés** : Les pods en phase **Succeeded** ou **Failed** (comme les Jobs nettoyés) ne bloquent pas le passage à **Unused=True**.
2. **Pods en attente (Pending)** : Un pod non encore planifié manifeste l'intention d'utiliser le volume. Le PVC conserve le statut **Unused=False** avec la raison **PodUsingPVC**.
3. **Volumes partagés (ReadWriteMany)** : Le statut bascule à **Unused=True** uniquement après l'arrêt du dernier pod consommateur.
4. **Champ lastTransitionTime** : Lors du passage à **Unused=True**, ce champ fige l'horodatage exact du début d'inactivité.

---

## 3. Conception d'un plugin kubectl en Go

Plutôt que de manipuler des requêtes JSON complexes, la création d'un binaire respectant les conventions des extensions **kubectl** permet d'intégrer ce suivi directement dans les routines d'exploitation.

### Principes d'intégration

1. **Convention de nommage** : Tout binaire nommé **kubectl-** disponible dans le **$PATH** système est automatiquement reconnu. La commande **kubectl pvc-usage** exécutera le binaire **kubectl-pvc-usage**.
2. **Gestion de la configuration** : L'utilisation de la bibliothèque officielle **k8s.io/cli-runtime/pkg/genericclioptions** garantit la prise en charge native des arguments standards (**--kubeconfig**, **--context**, **--namespace**, etc.).
3. **Arborescence de commandes avec Cobra** : Le paquet **[github.com/spf13/cobra](https://www.google.com/search?q=https%3A%2F%2Fgithub.com%2Fspf13%2Fcobra)** assure la gestion des flags et de l'aide en ligne.

---

## 4. Implémentation du plugin kubectl-pvc-usage

Structure du projet :

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

### 4.1. Configuration de cli-runtime et Cobra (pkg/cmd/root.go)

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

    // Drapeaux applicatifs spécifiques au plugin
    cmd.Flags().BoolVarP(&o.allNamespaces, "all-namespaces", "A", false, "Lister sur tous les namespaces")
    cmd.Flags().BoolVarP(&o.unusedOnly, "unused-only", "u", false, "Afficher uniquement les PVCs inutilisés")
    cmd.Flags().BoolVar(&o.inUseOnly, "in-use-only", false, "Afficher uniquement les PVCs en cours d'utilisation")
    cmd.Flags().StringVar(&o.minAgeStr, "min-age", "", "Filtrer les PVCs inutilisés depuis au moins cette durée (ex: '30d', '7d', '24h')")
    cmd.Flags().StringVar(&o.storageClass, "storage-class", "", "Filtrer par StorageClass")
    cmd.Flags().StringVar(&o.sortBy, "sort-by", "namespace", "Tri des résultats: 'namespace', 'name', 'unused', 'size', 'age'")
    cmd.Flags().BoolVar(&o.showPods, "show-pods", false, "Inspecter les pods connectés (mode wide)")
    cmd.Flags().StringVarP(&o.output, "output", "o", "table", "Format de sortie: 'table', 'wide', 'json', 'yaml'")
    cmd.Flags().BoolVar(&o.showSummary, "summary", true, "Afficher le bilan FinOps en fin de tableau")

    return cmd
}

```

### 4.2. Extraction de la condition (pkg/collector/collector.go)

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

### 4.3. Support des durées exprimées en jours (30d, 7d)

La fonction **time.ParseDuration** native de Go ne prenant pas en compte l'unité jour (**d**), le parser a été étendu :

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

### 4.4. Format de sortie terminal

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

## 5. Compilation multi-plateforme et packaging Krew

### Automation du build (Makefile)

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

### Manifeste Krew (krew.yaml)

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

### Modes d'installation via Krew

L'installation peut s'effectuer sans attendre l'intégration dans le dépôt officiel Krew.

#### Option 1 : Via flux shell

```bash
kubectl krew install --manifest=<(curl -fsSL https://raw.githubusercontent.com/herveleclerc/pvc-usage/main/krew.yaml)

```

#### Option 2 : Téléchargement du fichier de manifeste

```bash
curl -fsSLO https://raw.githubusercontent.com/herveleclerc/pvc-usage/main/krew.yaml
kubectl krew install --manifest=krew.yaml
rm -f krew.yaml

```

#### Option 3 : Ajout d'un index Git personnalisé

```bash
kubectl krew index add herveleclerc https://github.com/herveleclerc/pvc-usage.git
kubectl krew install herveleclerc/pvc-usage

```

---

## 6. Gestion des versions et rétrocompatibilité

Le binaire client **kubectl-pvc-usage** fonctionne indépendamment de la version locale de **kubectl**. En revanche, les données extraites dépendent directement de la version du plan de contrôle :

* **Kubernetes < 1.36** : Condition non gérée par le contrôleur.
* **Kubernetes 1.36 (Alpha)** : Requiert l'activation explicite du drapeau **--feature-gates="PersistentVolumeClaimUnusedSinceTime=true"** sur le **kube-controller-manager**.
* **Kubernetes 1.37+ (Beta)** : Activé par défaut.

### Vérification de compatibilité (pkg/versioncheck)

Le plugin interroge l'endpoint **/version** de l'API Server pour ajuster l'affichage et éviter les interprétations erronées sur des clusters plus anciens :

```go
package versioncheck

import (
    "fmt"
    "regexp"
    "strconv"

    "k8s.io/apimachinery/pkg/version"
    "k8s.io/client-go/discovery"
)

type CompatibilityStatus string

const (
    StatusFullySupported CompatibilityStatus = "FullySupported"
    StatusAlphaSupported CompatibilityStatus = "AlphaSupported"
    StatusUnsupported    CompatibilityStatus = "Unsupported"
)

func CheckServerVersion(discoveryClient discovery.ServerVersionInterface) (*ClusterCompatibility, error) {
    sv, err := discoveryClient.ServerVersion()
    if err != nil {
        return nil, fmt.Errorf("impossible de récupérer la version du serveur: %w", err)
    }
    return EvaluateVersion(sv), nil
}

func EvaluateVersion(sv *version.Info) *ClusterCompatibility {
    comp := &ClusterCompatibility{GitVersion: sv.GitVersion}
    minor, _ := strconv.Atoi(minorRegex.FindString(sv.Minor))
    comp.Minor = minor

    switch {
    case comp.Minor >= 37:
        comp.Status = StatusFullySupported
    case comp.Minor == 36:
        comp.Status = StatusAlphaSupported
        comp.Warning = fmt.Sprintf("Cluster en version %s (v1.36). 'PersistentVolumeClaimUnusedSinceTime' nécessite le feature gate activé sur kube-controller-manager.", sv.GitVersion)
    default:
        comp.Status = StatusUnsupported
        comp.Warning = fmt.Sprintf("Cluster en version %s (v1.%d). La condition 'Unused' requiert Kubernetes >= 1.37.", sv.GitVersion, comp.Minor)
    }
    return comp
}

```

Sur un cluster pré-1.37, le plugin envoie un avertissement sur **stderr** et indique **N/A (K8s < 1.37)** dans le tableau sans bloquer l'exécution.

---

## 7. Exemple d'utilisation sur un cluster 1.37

### 1. Déploiement du volume

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

### 2. Déploiement d'un pod associé

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

Contrôle de l'état :

```bash
$ kubectl pvc-usage -n demo -o wide --show-pods
NAMESPACE  NAME       STATUS  UNUSED-SINCE  CAPACITY  STORAGECLASS  VOLUME                                    ACCESSMODES    PODS      AGE  REASON
demo       test-data  InUse   Active        1Gi       standard      pvc-7bd3964b-3e0a-4478-9a00-8e460027273b  ReadWriteOnce  test-app  45s  PodUsingPVC

```

### 3. Suppression du pod

```bash
$ kubectl delete pod -n demo test-app

```

Vérification de la mise à jour par le contrôleur :

```bash
$ kubectl pvc-usage -n demo
NAMESPACE  NAME       STATUS  UNUSED-SINCE  CAPACITY  STORAGECLASS  AGE
demo       test-data  Unused  14s           1Gi       standard      4m

```

Filtrage des PVC inutilisés depuis plus de 30 jours à l'échelle du cluster :

```bash
$ kubectl pvc-usage -A --unused-only --min-age=30d --sort-by=unused

```

---

## 8. Résumé

L'introduction de la condition **Unused** dans Kubernetes 1.37 apporte une approche déclarative pour suivre le stockage inactif :

* L'état est directement exposé dans **.status.conditions**.
* L'horodatage **lastTransitionTime** permet d'automatiser des scripts de purge basés sur la durée d'inactivité réelle.
* L'utilisation du SDK **cli-runtime** simplifie le développement de plugins Go intégrés à l'écosystème nativement.

Le code source du plugin et ses tests sont disponibles sur GitHub :

[https://github.com/herveleclerc/pvc-usage](https://github.com/herveleclerc/pvc-usage?utm_source=gemini)