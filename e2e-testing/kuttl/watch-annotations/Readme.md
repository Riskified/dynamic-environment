## DEV-54056 Verifying Watch Annotations

This test verifies that all created resources have the watch annotation added
and managed correctly:

* Watch annotations are added to deployment
* Watch annotations are added to destination rules
* Watch annotations are added to virtual service upon modifications and removed
  upon rollback.
* Watch annotations persist after deployment modification
