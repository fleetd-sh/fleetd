package deployment

type EventBus interface{}
type DeviceRegistry interface{}
type DeploymentStore interface{}
type Executor interface{}

type DeploymentController struct {
	eventBus        EventBus
	deviceRegistry  DeviceRegistry
	deploymentStore DeploymentStore
	executors       map[string]Executor // oci, nixpacks, binary
}
