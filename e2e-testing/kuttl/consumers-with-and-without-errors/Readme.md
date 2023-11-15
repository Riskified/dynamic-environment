## Consumer Status (with and without errors)

This test verifies that consumer has its own section under _Status_, and it's not a part of
_Subsets_. This should apply both to regular status and error messages. We also verify the existence
of hash in consumers.

Also, it verifies that consumers are also deleted when the dynamic-environment is deleted.