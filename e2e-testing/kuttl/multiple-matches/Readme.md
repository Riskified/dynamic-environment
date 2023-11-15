## Multiple Istio Matches and Source Labels

This test validates the following features of multiple matches / source labels support:

* Order of the routes is equal to the order of matches
* Source labels defined in Subset are passed to the deployments launched for the subset.
