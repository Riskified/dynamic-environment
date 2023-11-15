# Multiple Services Scenarios (per single deployment)

Several scenarios of multiple services

* Step 0: multiple services with everything ok.
* Step 1+2: multiple services with one of two base destination rules missing - status should be
  running.
* Step 3+4: multiple services with all destination rules missing - status should be degraded with
  warnings.
* Step 5: Full virtual services and destination rules