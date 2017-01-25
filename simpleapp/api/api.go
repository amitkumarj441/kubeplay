package api

import (
	_ "fmt"
	"strings"

	unversioned "k8s.io/client-go/pkg/api/unversioned" // Should eventually migrate to "k8s.io/apimachinery/pkg/apis/meta/v1"?
	kapi "k8s.io/client-go/pkg/api/v1"
	kext "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/pkg/util/intstr"
)

// AppComponentOpts hold highlevel fields which map to a non-trivial settings
// within inside the object, often affecting sub-fields within sub-fields,
// for more trivial things (like hostNetwork) we have setters and getters
type AppComponentOpts struct {
	PrometheusPath   string
	PrometheusScrape bool
	// WithoutPorts implies ExcludeService and StandardProbes
	WithoutPorts                   bool
	WithoutStandardProbes          bool
	WithoutStandardSecurityContext bool
	HealthPath                     string
	LivenessPath                   string
	// XXX we can add these here, but may be they belong elsewhere?
	//WithProbes interface{}
	//WithSecurityContext interface{}
	// WithoutService disables building of the service
	WithoutService bool
}

type AppComponent struct {
	Image    string
	Name     string
	Port     int32
	Replicas *int32
	Opts     AppComponentOpts
	Env      map[string]string
	// It's probably okay for now, but we'd eventually want to
	// inherit properties defined outside of the AppComponent struct,
	// that it anything we'd use setters and getters for, so we might
	// want to figure out intermediate struct or just write more
	// some tests to see how things would work without that...
	BasedOn *AppComponent
}

// Global defaults
const (
	DEFAULT_REPLICAS = int32(1)
	DEFAULT_PORT     = int32(80)
)

// Everything we want to controll per-app
type AppComponentBuildOpts struct {
	Namespace              string
	DefaultReplicas        int32
	DefaultPort            int32
	StandardLivenessProbe  *kapi.Probe
	StandardReadinessProbe *kapi.Probe
}

type App struct {
	Name  string
	Group []AppComponent
}

// TODO figure out how to use kapi.List here, if we can
// TODO find a way to use something other then Deployment
// e.g. StatefullSet or DaemonSet, also attach a ConfigMap
// or a secret or several of those things
type AppComponentResourcePair struct {
	Deployment *kext.Deployment
	Service    *kapi.Service
}

func (i *AppComponent) GetNameAndLabels() (string, map[string]string) {
	var name string

	imageParts := strings.Split(strings.Split(i.Image, ":")[0], "/")
	name = imageParts[len(imageParts)-1]

	if i.Name != "" {
		name = i.Name
	}

	labels := map[string]string{"name": name}

	return name, labels
}

func (i *AppComponent) GetMeta() kapi.ObjectMeta {
	name, labels := i.GetNameAndLabels()
	return kapi.ObjectMeta{
		Name:   name,
		Labels: labels,
	}
}

func (i *AppComponent) BuildContainer(opts AppComponentBuildOpts) kapi.Container {
	name, _ := i.GetNameAndLabels()
	container := kapi.Container{Name: name, Image: i.Image}

	env := []kapi.EnvVar{}
	for k, v := range i.Env {
		env = append(env, kapi.EnvVar{Name: k, Value: v})
	}
	if len(env) > 0 {
		container.Env = env
	}

	port := kapi.ContainerPort{
		Name:          name,
		ContainerPort: opts.DefaultPort,
	}
	if i.Port != 0 {
		port.ContainerPort = i.Port
	}
	container.Ports = []kapi.ContainerPort{port}

	return container
}

func (i *AppComponent) getPort(opts AppComponentBuildOpts) int32 {
	if i.Port != 0 {
		return i.Port
	}
	return opts.DefaultPort
}

func (i *AppComponent) maybeAddProbes(opts AppComponentBuildOpts, container *kapi.Container) {
	if i.Opts.WithoutStandardProbes {
		return
	}
	port := intstr.FromInt(int(i.getPort(opts)))

	container.ReadinessProbe = &kapi.Probe{
		PeriodSeconds:       3,
		InitialDelaySeconds: 180,
		Handler: kapi.Handler{
			HTTPGet: &kapi.HTTPGetAction{
				Path: "/health",
				Port: port,
			},
		},
	}
	container.LivenessProbe = &kapi.Probe{
		PeriodSeconds:       3,
		InitialDelaySeconds: 300,
		Handler: kapi.Handler{
			HTTPGet: &kapi.HTTPGetAction{
				Path: "/health",
				Port: port,
			},
		},
	}
}

func (i *AppComponent) BuildPod(opts AppComponentBuildOpts) *kapi.PodTemplateSpec {
	name, labels := i.GetNameAndLabels()
	container := kapi.Container{Name: name, Image: i.Image}

	env := []kapi.EnvVar{}
	for k, v := range i.Env {
		env = append(env, kapi.EnvVar{Name: k, Value: v})
	}
	if len(env) > 0 {
		container.Env = env
	}

	port := kapi.ContainerPort{ContainerPort: i.getPort(opts)}
	container.Ports = []kapi.ContainerPort{port}

	i.maybeAddProbes(opts, &container)

	pod := kapi.PodTemplateSpec{
		ObjectMeta: kapi.ObjectMeta{
			Labels: labels,
		},
		Spec: kapi.PodSpec{
			Containers: []kapi.Container{container},
		},
	}

	return &pod
}

func (i *AppComponentResourcePair) AppendContainer(container kapi.Container) AppComponentResourcePair {
	containers := &i.Deployment.Spec.Template.Spec.Containers
	*containers = append(*containers, container)
	return *i
}

func (i *AppComponentResourcePair) MountDataVolume() AppComponentResourcePair {
	// TODO append to volumes and volume mounts based on few simple parameters
	// when user uses more then one container, they will have to do it in a low-level way
	// secrets and config maps would be handled separatelly, so we call this MountDataVolume()
	// and not something else
	return *i
}

func (i *AppComponentResourcePair) WithSecret(secretData interface{}) AppComponentResourcePair {
	return *i
}

func (i *AppComponentResourcePair) WithConfig(configMapData interface{}) AppComponentResourcePair {
	return *i
}

func (i *AppComponentResourcePair) SetHostNetwork(bool) AppComponentResourcePair {
	return *i
}

func (i *AppComponentResourcePair) GetHostNetwork() AppComponentResourcePair {
	return *i
}

func (i *AppComponentResourcePair) SetHostPID(bool) AppComponentResourcePair {
	return *i
}

func (i *AppComponentResourcePair) GetHostPID() AppComponentResourcePair {
	return *i
}

func (i *AppComponent) BuildDeployment(opts AppComponentBuildOpts, pod *kapi.PodTemplateSpec) *kext.Deployment {
	if pod == nil {
		return nil
	}

	meta := i.GetMeta()

	replicas := opts.DefaultReplicas

	if i.Replicas != nil {
		replicas = *i.Replicas
	}

	deploymentSpec := kext.DeploymentSpec{
		Replicas: &replicas,
		Selector: &unversioned.LabelSelector{MatchLabels: meta.Labels},
		Template: *pod,
	}

	deployment := &kext.Deployment{
		ObjectMeta: meta,
		Spec:       deploymentSpec,
	}

	if opts.Namespace != "" {
		deployment.ObjectMeta.Namespace = opts.Namespace
	}

	return deployment
}

func (i *AppComponent) BuildService(opts AppComponentBuildOpts) *kapi.Service {
	meta := i.GetMeta()

	port := kapi.ServicePort{Port: i.getPort(opts)}
	if i.Port != 0 {
		port.Port = i.Port
	}

	service := &kapi.Service{
		ObjectMeta: meta,
		Spec: kapi.ServiceSpec{
			Ports:    []kapi.ServicePort{port},
			Selector: meta.Labels,
		},
	}

	return service
}

func (i *AppComponent) Build(opts AppComponentBuildOpts) AppComponentResourcePair {
	pod := i.BuildPod(opts)

	return AppComponentResourcePair{
		i.BuildDeployment(opts, pod),
		i.BuildService(opts),
	}
}

func (i *App) Build() []AppComponentResourcePair {
	opts := AppComponentBuildOpts{
		Namespace:       i.Name,
		DefaultReplicas: DEFAULT_REPLICAS,
		DefaultPort:     DEFAULT_PORT,
		// standardSecurityContext
		// standardTmpVolume?
	}

	list := []AppComponentResourcePair{}

	for _, service := range i.Group {
		list = append(list, service.Build(opts))
	}

	return list
}
