## DEV-55209 VirtualService with multiple services bug

This bug happens when more than one subset is handled by the same virtual service. Once we handle it
in the first subset, we consider it as done and don't touch it again in the second subset which
should handle a different section.

> Note: you cannot test this app in a browser—in order to merge all *VirtualService*s I added an
> invalid *uri path*.

In this test, we verify that:

* Both `details` and `reviews` are modified in the single `VirtualService`.