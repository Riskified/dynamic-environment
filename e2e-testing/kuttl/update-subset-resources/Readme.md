## DEV-55206 - Testing update subset resources

Verify that we can update certain fields inside each subset. We also test that the hashes are
updating.

> Note, because of the way Kuttl queries the cluster we can not check intermediate status
> (`updating`). There result is unstable because the status is updated quickly.

