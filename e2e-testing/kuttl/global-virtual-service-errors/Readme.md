## Global Virtual Service Errors

This test makes sure we add global virtual service errors when needed. It only tests that the
provisioning for adding global errors works, It does not test the various use cases in which we add
these errors. It also validates that the *state* is *degraded* if such error exists.