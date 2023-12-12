## Add and Remove Subsets and Consumers

This test validates we can add and remove subsets and consumers at will. Workflow includes:

* Step 0: Launch with single subset.
* Step 1: Add second subset and a consumer.
* Step 3: Remove first subset and consumer.

(other steps are for validation).

> Note: Currently in Kuttl I can not verify that a subset *does not* exist in the status. Consider
> adding `TestAssert` with `Command` to somehow verify this (or use other means to verify).