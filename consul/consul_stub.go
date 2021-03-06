package consul

import (
	"fmt"
	"sync"

	"github.com/allegro/marathon-consul/apps"
	"github.com/allegro/marathon-consul/service"
	consulapi "github.com/hashicorp/consul/api"
)

// TODO this should be a service registry stub in the service package, requires abstracting from AgentServiceRegistration
type Stub struct {
	sync.RWMutex
	services                   map[service.ID]*consulapi.AgentServiceRegistration
	failGetServicesForNames    map[string]bool
	failRegisterForIDs         map[apps.TaskID]bool
	failDeregisterByTaskForIDs map[apps.TaskID]bool
	failDeregisterForIDs       map[service.ID]bool
	consul                     *Consul
}

func NewConsulStub() *Stub {
	return NewConsulStubWithTag("marathon")
}

func NewConsulStubWithTag(tag string) *Stub {
	return &Stub{
		services:                   make(map[service.ID]*consulapi.AgentServiceRegistration),
		failGetServicesForNames:    make(map[string]bool),
		failRegisterForIDs:         make(map[apps.TaskID]bool),
		failDeregisterByTaskForIDs: make(map[apps.TaskID]bool),
		failDeregisterForIDs:       make(map[service.ID]bool),
		consul:                     New(Config{Tag: tag, ConsulNameSeparator: "."}),
	}
}

func (c *Stub) GetAllServices() ([]*service.Service, error) {
	c.RLock()
	defer c.RUnlock()
	var allServices []*service.Service
	for _, s := range c.services {
		allServices = append(allServices, &service.Service{
			ID:           service.ID(s.ID),
			Name:         s.Name,
			Tags:         s.Tags,
			AgentAddress: s.Address,
		})
	}
	return allServices, nil
}

func (c *Stub) FailGetServicesForName(failOnName string) {
	c.failGetServicesForNames[failOnName] = true
}

func (c *Stub) FailRegisterForID(taskID apps.TaskID) {
	c.failRegisterForIDs[taskID] = true
}

func (c *Stub) FailDeregisterByTaskForID(taskID apps.TaskID) {
	c.failDeregisterByTaskForIDs[taskID] = true
}

func (c *Stub) FailDeregisterForID(serviceID service.ID) {
	c.failDeregisterForIDs[serviceID] = true
}

func (c *Stub) GetServices(name string) ([]*service.Service, error) {
	c.RLock()
	defer c.RUnlock()
	if _, ok := c.failGetServicesForNames[name]; ok {
		return nil, fmt.Errorf("Consul stub programmed to fail when getting services for name %s", name)
	}
	var services []*service.Service
	for _, s := range c.services {
		if s.Name == name && contains(s.Tags, c.consul.config.Tag) {
			services = append(services, &service.Service{
				ID:           service.ID(s.ID),
				Name:         s.Name,
				Tags:         s.Tags,
				AgentAddress: s.Address,
			})
		}
	}
	return services, nil
}

func (c *Stub) Register(task *apps.Task, app *apps.App) error {
	c.Lock()
	defer c.Unlock()
	if _, ok := c.failRegisterForIDs[task.ID]; ok {
		return fmt.Errorf("Consul stub programmed to fail when registering task of id %s", task.ID.String())
	}
	serviceRegistrations, err := c.consul.marathonTaskToConsulServices(task, app)
	if err != nil {
		return err
	}
	for _, r := range serviceRegistrations {
		c.services[service.ID(r.ID)] = r
	}
	return nil
}

func (c *Stub) RegisterWithoutMarathonTaskTag(task *apps.Task, app *apps.App) {
	c.Lock()
	defer c.Unlock()
	for _, intent := range app.RegistrationIntents(task, c.consul.config.ConsulNameSeparator) {
		serviceRegistration := consulapi.AgentServiceRegistration{
			ID:      task.ID.String(),
			Name:    intent.Name,
			Port:    intent.Port,
			Address: task.Host,
			Tags:    intent.Tags,
			Checks:  consulapi.AgentServiceChecks{},
		}
		c.services[service.ID(serviceRegistration.ID)] = &serviceRegistration
	}
}

func (c *Stub) RegisterOnlyFirstRegistrationIntent(task *apps.Task, app *apps.App) {
	c.Lock()
	defer c.Unlock()
	serviceRegistrations, _ := c.consul.marathonTaskToConsulServices(task, app)
	c.services[service.ID(serviceRegistrations[0].ID)] = serviceRegistrations[0]
}

func (c *Stub) DeregisterByTask(taskID apps.TaskID) error {
	c.Lock()
	defer c.Unlock()
	if _, ok := c.failDeregisterByTaskForIDs[taskID]; ok {
		return fmt.Errorf("Consul stub programmed to fail when deregistering task of id %s", taskID.String())
	}
	for _, x := range c.servicesMatchingTask(taskID) {
		delete(c.services, service.ID(x.ID))
	}
	return nil
}

func (c *Stub) Deregister(toDeregister *service.Service) error {
	c.Lock()
	defer c.Unlock()
	if _, ok := c.failDeregisterForIDs[toDeregister.ID]; ok {
		return fmt.Errorf("Consul stub programmed to fail when deregistering service of id %s", toDeregister.ID)
	}
	delete(c.services, toDeregister.ID)
	return nil
}

func (c *Stub) servicesMatchingTask(taskID apps.TaskID) []*consulapi.AgentServiceRegistration {
	matching := []*consulapi.AgentServiceRegistration{}
	for _, s := range c.services {
		if s.ID == taskID.String() || contains(s.Tags, fmt.Sprintf("marathon-task:%s", taskID.String())) {
			matching = append(matching, s)
		}
	}
	return matching
}

func (c *Stub) RegisteredTaskIDs(serviceName string) []apps.TaskID {
	services, _ := c.GetServices(serviceName)
	taskIds := []apps.TaskID{}
	for _, s := range services {
		taskID, _ := s.TaskID()
		taskIds = append(taskIds, taskID)
	}
	return taskIds
}
