## DEV-55206 - Testing update subset resources

Verify that we can update certain fields inside each subset. We also test that the hashes are
 updated.

> Note, because of the way Kuttl queries the cluster we cannot check intermediate status
> (`updating`). The result is unstable because the status is updated quickly.

