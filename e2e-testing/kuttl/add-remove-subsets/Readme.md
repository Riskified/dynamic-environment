## Add and Remove Subsets and Consumers

This test validates we can add and remove subsets and consumers at will. Workflow includes:

* Step 0: Launch with single subset.
* Step 1: Add second subset and a consumer.
* Step 3: Remove first subset and consumer.

(other steps are for validation).

Using `yq` to validate that removed subsets/consumer does not exist in the status after removal.