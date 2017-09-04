package rubykube

import (
	"fmt"
	"strings"

	mruby "github.com/mitchellh/go-mruby"
	kapi "k8s.io/client-go/pkg/api/v1"
	kext "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

type deploymentTypeAlias kext.Deployment

//go:generate gotemplate "./templates/resource" "deploymentClass(\"Deployment\", deployment, deploymentTypeAlias)"

func (c *deploymentClass) getSingleton(ns, name string) (*kext.Deployment, error) {
	return c.rk.clientset.Extensions().Deployments(ns).Get(name)
}

//go:generate gotemplate "./templates/resource/singleton" "deploymentSingletonModule(deploymentClass, \"deployment\", deployment, deploymentTypeAlias)"

//go:generate gotemplate "./templates/resource/podfinder" "deploymentPodFinderModule(deploymentClass, \"deployment\", deployment, deploymentTypeAlias)"

func (c *deploymentClass) defineOwnMethods() {
	c.defineSingletonMethods()
	c.definePodFinderMethods()

	c.rk.appendMethods(c.class, map[string]methodDefintion{
		"replicasets": {
			mruby.ArgsNone(), func(m *mruby.Mrb, self *mruby.MrbValue) (mruby.Value, mruby.Value) {
				vars, err := c.LookupVars(self)
				if err != nil {
					return nil, createException(m, err.Error())
				}

				ns := vars.deployment.ObjectMeta.Namespace

				selector := []string{}
				// TODO: probably should use `spec.selector`
				for k, v := range vars.deployment.ObjectMeta.Labels {
					selector = append(selector, fmt.Sprintf("%s in (%s)", k, v))
				}
				listOptions := kapi.ListOptions{LabelSelector: strings.Join(selector, ",")}

				replicaSets, err := c.rk.clientset.Extensions().ReplicaSets(ns).List(listOptions)
				if err != nil {
					return nil, createException(m, err.Error())
				}

				newReplicaSetsObj, err := c.rk.classes.ReplicaSets.New()
				if err != nil {
					return nil, createException(m, err.Error())
				}
				newReplicaSetsObj.vars.replicaSets = replicaSetListTypeAlias(*replicaSets)
				return newReplicaSetsObj.self, nil
			},
			instanceMethod,
		},
	})
}

func (o *deploymentClassInstance) Update() (mruby.Value, error) {
	return call(o.self, "get!")
}