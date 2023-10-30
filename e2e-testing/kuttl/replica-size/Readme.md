## Correct replica size

This test verifies that when creating *DynamicEnv* it will create the correct *Deployment* replica
size. If specified it will set the specified number of replicas, of not then the default (1). It
checks on both consumers and subsets (creation and updates).


> Note: the original test also contains validating of _DestinationRule_. Not really related for this
> test, but I'll leave it anyway. In case the destination rule test fails you can probably delete it
(but you have to _figure out why and make sure you have alternative test_).
