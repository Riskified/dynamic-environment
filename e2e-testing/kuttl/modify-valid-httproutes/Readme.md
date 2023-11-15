## DEV-46154 Verify That Only Valid HttpRoutes are Modified

This test verifies that when updating *VirtualService* we do not modify routes
that are pointing to different service then hours (you can see the comments in
the `01-assert.yaml` file). It then verifies that removing the *DynamicEnv* the
virtual service return to its original state.

### Notes

* The original `00-virtual-service.yaml` file would probably fail to install in
  an environment that contains full *Istio* because it points to invalid target.
* The modified routes names in `01-assert.yaml` are commented out as *Kuttl*
  only supports full match (no wildcard) and every time we'll update the test we
  may change the hashed string and break the test.
