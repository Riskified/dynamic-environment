## DEV-46153 Correct state

This test verifies that the *DynamicEnv* contains all created *Deployment*, *DestinationRule* and
*VirtualService*.

> Note: we do not test for empty subset errors as Kuttl doesn't seem to support testing the omission
> of a key regardless of its value.
