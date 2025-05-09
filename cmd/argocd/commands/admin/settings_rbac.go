package admin

import (
	"context"
	"fmt"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"

	"github.com/argoproj/argo-cd/v3/common"
	"github.com/argoproj/argo-cd/v3/util/assets"
	"github.com/argoproj/argo-cd/v3/util/cli"
	"github.com/argoproj/argo-cd/v3/util/rbac"
)

type actionTraitMap map[string]rbacTrait

type rbacTrait struct {
	allowPath bool
}

// Provide a mapping of shorthand resource names to their RBAC counterparts
var resourceMap = map[string]string{
	"account":         rbac.ResourceAccounts,
	"app":             rbac.ResourceApplications,
	"apps":            rbac.ResourceApplications,
	"application":     rbac.ResourceApplications,
	"applicationsets": rbac.ResourceApplicationSets,
	"cert":            rbac.ResourceCertificates,
	"certs":           rbac.ResourceCertificates,
	"certificate":     rbac.ResourceCertificates,
	"cluster":         rbac.ResourceClusters,
	"extension":       rbac.ResourceExtensions,
	"gpgkey":          rbac.ResourceGPGKeys,
	"key":             rbac.ResourceGPGKeys,
	"log":             rbac.ResourceLogs,
	"logs":            rbac.ResourceLogs,
	"exec":            rbac.ResourceExec,
	"proj":            rbac.ResourceProjects,
	"projs":           rbac.ResourceProjects,
	"project":         rbac.ResourceProjects,
	"repo":            rbac.ResourceRepositories,
	"repos":           rbac.ResourceRepositories,
	"repository":      rbac.ResourceRepositories,
}

// List of allowed RBAC resources
var validRBACResourcesActions = map[string]actionTraitMap{
	rbac.ResourceAccounts:        accountsActions,
	rbac.ResourceApplications:    applicationsActions,
	rbac.ResourceApplicationSets: defaultCRUDActions,
	rbac.ResourceCertificates:    defaultCRDActions,
	rbac.ResourceClusters:        defaultCRUDActions,
	rbac.ResourceExtensions:      extensionActions,
	rbac.ResourceGPGKeys:         defaultCRDActions,
	rbac.ResourceLogs:            logsActions,
	rbac.ResourceExec:            execActions,
	rbac.ResourceProjects:        defaultCRUDActions,
	rbac.ResourceRepositories:    defaultCRUDActions,
}

// List of allowed RBAC actions
var defaultCRUDActions = actionTraitMap{
	rbac.ActionCreate: rbacTrait{},
	rbac.ActionGet:    rbacTrait{},
	rbac.ActionUpdate: rbacTrait{},
	rbac.ActionDelete: rbacTrait{},
}

var defaultCRDActions = actionTraitMap{
	rbac.ActionCreate: rbacTrait{},
	rbac.ActionGet:    rbacTrait{},
	rbac.ActionDelete: rbacTrait{},
}

var applicationsActions = actionTraitMap{
	rbac.ActionCreate:   rbacTrait{},
	rbac.ActionGet:      rbacTrait{},
	rbac.ActionUpdate:   rbacTrait{allowPath: true},
	rbac.ActionDelete:   rbacTrait{allowPath: true},
	rbac.ActionAction:   rbacTrait{allowPath: true},
	rbac.ActionOverride: rbacTrait{},
	rbac.ActionSync:     rbacTrait{},
}

var accountsActions = actionTraitMap{
	rbac.ActionCreate: rbacTrait{},
	rbac.ActionUpdate: rbacTrait{},
}

var execActions = actionTraitMap{
	rbac.ActionCreate: rbacTrait{},
}

var logsActions = actionTraitMap{
	rbac.ActionGet: rbacTrait{},
}

var extensionActions = actionTraitMap{
	rbac.ActionInvoke: rbacTrait{},
}

// NewRBACCommand is the command for 'rbac'
func NewRBACCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "rbac",
		Short: "Validate and test RBAC configuration",
		Run: func(c *cobra.Command, args []string) {
			c.HelpFunc()(c, args)
		},
	}
	command.AddCommand(NewRBACCanCommand())
	command.AddCommand(NewRBACValidateCommand())
	return command
}

// NewRBACCanCommand is the command for 'rbac can'
func NewRBACCanCommand() *cobra.Command {
	var (
		policyFile   string
		defaultRole  string
		useBuiltin   bool
		strict       bool
		quiet        bool
		subject      string
		action       string
		resource     string
		subResource  string
		clientConfig clientcmd.ClientConfig
	)
	command := &cobra.Command{
		Use:   "can ROLE/SUBJECT ACTION RESOURCE [SUB-RESOURCE]",
		Short: "Check RBAC permissions for a role or subject",
		Long: `
Check whether a given role or subject has appropriate RBAC permissions to do
something.
`,
		Example: `
# Check whether role some:role has permissions to create an application in the
# 'default' project, using a local policy.csv file
argocd admin settings rbac can some:role create application 'default/app' --policy-file policy.csv

# Policy file can also be K8s config map with data keys like argocd-rbac-cm,
# i.e. 'policy.csv' and (optionally) 'policy.default'
argocd admin settings rbac can some:role create application 'default/app' --policy-file argocd-rbac-cm.yaml

# If --policy-file is not given, the ConfigMap 'argocd-rbac-cm' from K8s is
# used. You need to specify the argocd namespace, and make sure that your
# current Kubernetes context is pointing to the cluster Argo CD is running in
argocd admin settings rbac can some:role create application 'default/app' --namespace argocd

# You can override a possibly configured default role
argocd admin settings rbac can someuser create application 'default/app' --default-role role:readonly

`,
		Run: func(c *cobra.Command, args []string) {
			ctx := c.Context()

			if len(args) < 3 || len(args) > 4 {
				c.HelpFunc()(c, args)
				os.Exit(1)
			}
			subject = args[0]
			action = args[1]
			resource = args[2]
			if len(args) > 3 {
				subResource = args[3]
			}

			namespace, nsOverride, err := clientConfig.Namespace()
			if err != nil {
				log.Fatalf("could not create k8s client: %v", err)
			}

			// Exactly one of --namespace or --policy-file must be given.
			if (!nsOverride && policyFile == "") || (nsOverride && policyFile != "") {
				c.HelpFunc()(c, args)
				log.Fatalf("please provide exactly one of --policy-file or --namespace")
			}

			restConfig, err := clientConfig.ClientConfig()
			if err != nil {
				log.Fatalf("could not create k8s client: %v", err)
			}
			realClientset, err := kubernetes.NewForConfig(restConfig)
			if err != nil {
				log.Fatalf("could not create k8s client: %v", err)
			}

			userPolicy, newDefaultRole, matchMode := getPolicy(ctx, policyFile, realClientset, namespace)

			// Use built-in policy as augmentation if requested
			builtinPolicy := ""
			if useBuiltin {
				builtinPolicy = assets.BuiltinPolicyCSV
			}

			// If no explicit default role was given, but we have one defined from
			// a policy, use this to check for enforce.
			if newDefaultRole != "" && defaultRole == "" {
				defaultRole = newDefaultRole
			}

			res := checkPolicy(subject, action, resource, subResource, builtinPolicy, userPolicy, defaultRole, matchMode, strict)
			if res {
				if !quiet {
					fmt.Println("Yes")
				}
				os.Exit(0)
			}
			if !quiet {
				fmt.Println("No")
			}
			os.Exit(1)
		},
	}
	clientConfig = cli.AddKubectlFlagsToCmd(command)
	command.Flags().StringVar(&policyFile, "policy-file", "", "path to the policy file to use")
	command.Flags().StringVar(&defaultRole, "default-role", "", "name of the default role to use")
	command.Flags().BoolVar(&useBuiltin, "use-builtin-policy", true, "whether to also use builtin-policy")
	command.Flags().BoolVar(&strict, "strict", true, "whether to perform strict check on action and resource names")
	command.Flags().BoolVarP(&quiet, "quiet", "q", false, "quiet mode - do not print results to stdout")
	return command
}

// NewRBACValidateCommand returns a new rbac validate command
func NewRBACValidateCommand() *cobra.Command {
	var (
		policyFile   string
		namespace    string
		clientConfig clientcmd.ClientConfig
	)

	command := &cobra.Command{
		Use:   "validate [--policy-file POLICYFILE] [--namespace NAMESPACE]",
		Short: "Validate RBAC policy",
		Long: `
Validates an RBAC policy for being syntactically correct. The policy must be
a local file or a K8s ConfigMap in the provided namespace, and in either CSV or K8s ConfigMap format.
`,
		Example: `
# Check whether a given policy file is valid using a local policy.csv file.
argocd admin settings rbac validate --policy-file policy.csv

# Policy file can also be K8s config map with data keys like argocd-rbac-cm,
# i.e. 'policy.csv' and (optionally) 'policy.default'
argocd admin settings rbac validate --policy-file argocd-rbac-cm.yaml

# If --policy-file is not given, and instead --namespace is giventhe ConfigMap 'argocd-rbac-cm'
# from K8s is used.
argocd admin settings rbac validate --namespace argocd

# Either --policy-file or --namespace must be given.
`,
		Run: func(c *cobra.Command, args []string) {
			ctx := c.Context()

			if len(args) > 0 {
				c.HelpFunc()(c, args)
				log.Fatalf("too many arguments")
			}

			if (namespace == "" && policyFile == "") || (namespace != "" && policyFile != "") {
				c.HelpFunc()(c, args)
				log.Fatalf("please provide exactly one of --policy-file or --namespace")
			}

			restConfig, err := clientConfig.ClientConfig()
			if err != nil {
				log.Fatalf("could not get config to create k8s client: %v", err)
			}
			realClientset, err := kubernetes.NewForConfig(restConfig)
			if err != nil {
				log.Fatalf("could not create k8s client: %v", err)
			}

			userPolicy, _, _ := getPolicy(ctx, policyFile, realClientset, namespace)
			if userPolicy != "" {
				if err := rbac.ValidatePolicy(userPolicy); err == nil {
					fmt.Printf("Policy is valid.\n")
					os.Exit(0)
				}
				fmt.Printf("Policy is invalid: %v\n", err)
				os.Exit(1)
			}
			log.Fatalf("Policy is empty or could not be loaded.")
		},
	}
	clientConfig = cli.AddKubectlFlagsToCmd(command)
	command.Flags().StringVar(&policyFile, "policy-file", "", "path to the policy file to use")
	command.Flags().StringVar(&namespace, "namespace", "", "namespace to get argo rbac configmap from")

	return command
}

// Load user policy file if requested or use Kubernetes client to get the
// appropriate ConfigMap from the current context
func getPolicy(ctx context.Context, policyFile string, kubeClient kubernetes.Interface, namespace string) (userPolicy string, defaultRole string, matchMode string) {
	var err error
	if policyFile != "" {
		// load from file
		userPolicy, defaultRole, matchMode, err = getPolicyFromFile(policyFile)
		if err != nil {
			log.Fatalf("could not read policy file: %v", err)
		}
	} else {
		cm, err := getPolicyConfigMap(ctx, kubeClient, namespace)
		if err != nil {
			log.Fatalf("could not get configmap: %v", err)
		}
		userPolicy, defaultRole, matchMode = getPolicyFromConfigMap(cm)
	}

	return userPolicy, defaultRole, matchMode
}

// getPolicyFromFile loads a RBAC policy from given path
func getPolicyFromFile(policyFile string) (string, string, string, error) {
	var (
		userPolicy  string
		defaultRole string
		matchMode   string
	)

	upol, err := os.ReadFile(policyFile)
	if err != nil {
		log.Fatalf("error opening policy file: %v", err)
		return "", "", "", err
	}

	// Try to unmarshal the input file as ConfigMap first. If it succeeds, we
	// assume config map input. Otherwise, we treat it as
	var upolCM *corev1.ConfigMap
	err = yaml.Unmarshal(upol, &upolCM)
	if err != nil {
		userPolicy = string(upol)
	} else {
		userPolicy, defaultRole, matchMode = getPolicyFromConfigMap(upolCM)
	}

	return userPolicy, defaultRole, matchMode, nil
}

// Retrieve policy information from a ConfigMap
func getPolicyFromConfigMap(cm *corev1.ConfigMap) (string, string, string) {
	var (
		defaultRole string
		ok          bool
	)

	defaultRole, ok = cm.Data[rbac.ConfigMapPolicyDefaultKey]
	if !ok {
		defaultRole = ""
	}

	return rbac.PolicyCSV(cm.Data), defaultRole, cm.Data[rbac.ConfigMapMatchModeKey]
}

// getPolicyConfigMap fetches the RBAC config map from K8s cluster
func getPolicyConfigMap(ctx context.Context, client kubernetes.Interface, namespace string) (*corev1.ConfigMap, error) {
	cm, err := client.CoreV1().ConfigMaps(namespace).Get(ctx, common.ArgoCDRBACConfigMapName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return cm, nil
}

// checkPolicy checks whether given subject is allowed to execute specified
// action against specified resource
func checkPolicy(subject, action, resource, subResource, builtinPolicy, userPolicy, defaultRole, matchMode string, strict bool) bool {
	enf := rbac.NewEnforcer(nil, "argocd", "argocd-rbac-cm", nil)
	enf.SetDefaultRole(defaultRole)
	enf.SetMatchMode(matchMode)
	if builtinPolicy != "" {
		if err := enf.SetBuiltinPolicy(builtinPolicy); err != nil {
			log.Fatalf("could not set built-in policy: %v", err)
			return false
		}
	}
	if userPolicy != "" {
		if err := rbac.ValidatePolicy(userPolicy); err != nil {
			log.Fatalf("invalid user policy: %v", err)
			return false
		}
		if err := enf.SetUserPolicy(userPolicy); err != nil {
			log.Fatalf("could not set user policy: %v", err)
			return false
		}
	}

	// User could have used a mutation of the resource name (i.e. 'cert' for
	// 'certificate') - let's resolve it to the valid resource.
	realResource := resolveRBACResourceName(resource)

	// If in strict mode, validate that given RBAC resource and action are
	// actually valid tokens.
	if strict {
		if err := validateRBACResourceAction(realResource, action); err != nil {
			log.Fatalf("error in RBAC request: %v", err)
			return false
		}
	}

	// Some project scoped resources have a special notation - for simplicity's sake,
	// if user gives no sub-resource (or specifies simple '*'), we construct
	// the required notation by setting subresource to '*/*'.
	if rbac.ProjectScoped[realResource] {
		if subResource == "*" || subResource == "" {
			subResource = "*/*"
		}
	}
	return enf.Enforce(subject, realResource, action, subResource)
}

// resolveRBACResourceName resolves a user supplied value to a valid RBAC
// resource name. If no mapping is found, returns the value verbatim.
func resolveRBACResourceName(name string) string {
	if res, ok := resourceMap[name]; ok {
		return res
	}
	return name
}

// validateRBACResourceAction checks whether a given resource is a valid RBAC resource.
// If it is, it validates that the action is a valid RBAC action for this resource.
func validateRBACResourceAction(resource, action string) error {
	validActions, ok := validRBACResourcesActions[resource]
	if !ok {
		return fmt.Errorf("'%s' is not a valid resource name", resource)
	}

	realAction, _, hasPath := strings.Cut(action, "/")
	actionTrait, ok := validActions[realAction]
	if !ok || hasPath && !actionTrait.allowPath {
		return fmt.Errorf("'%s' is not a valid action for %s", action, resource)
	}
	return nil
}
