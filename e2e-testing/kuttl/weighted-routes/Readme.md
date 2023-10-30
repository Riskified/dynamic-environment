# Testing DynamicEnv in Weighted Routes Scenarios

In this test we're testing two cases of weighted scenarios (e.g. could happen
during the progress of canary deploy):

1. This scenario is when one route is 100% and the other is 0% (e.g. before or
   after canary deploy).
2. This scenario is when both routes have some weight (e.g. in the middle of
   deploy). These weights should be kept in existing routes.

Note, This test currently does not handle modifications during deploy as we do
not yet support it in our code.
