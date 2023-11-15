## DEV-52636 - Support Delegated Virtual Services

This test verified integration with delegate virtual service:

- Test virtual service with VS that contains both delegated and direct routes.
- Test VS across multiple namespaces.
- Test correct status
- Test cleanup of virtual *all* virtual services (delegated and direct)
- Test status errors (produced by missing delegate - we inject a delegated route to missing virtual
  service)
