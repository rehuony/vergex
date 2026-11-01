package agent

// providerRegistry maps provider names to factory functions.
var providerRegistry = map[string]func(...ClientOption) Client{}

// RegisterProvider registers a provider factory function.
// Called by provider/payment sub-packages in their init() functions.
func RegisterProvider(name string, factory func(...ClientOption) Client) {
	providerRegistry[name] = factory
}

// NewClientByProvider creates an Client by provider name using the registry.
// Returns nil if the provider is not registered.
func NewClientByProvider(name string, opts ...ClientOption) Client {
	factory, ok := providerRegistry[name]
	if !ok {
		return nil
	}
	return factory(opts...)
}
