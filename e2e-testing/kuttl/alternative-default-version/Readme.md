## Support Custom Default Version per Subset

This test is for validating part of the effort to support custom version labels
and default values. We do not test for global custom version label (this is only
global) or global value as it will affect the entire end-to-end testing. We only
test the option to add specific custom default value per subset.

For validation, we:

* Validate that a _DestinationRule_ was created for the subset with custom
  default version (_reviews_).
* Validate the rules of the _VirtualService_ for set service.
