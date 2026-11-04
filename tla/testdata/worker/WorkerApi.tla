---- MODULE WorkerApi ----
\* The Worker as its callers see it: jobs are accepted while it is open, every
\* accepted job runs, and it stops only once it is closed and nothing is
\* pending. Worker.tla refines this module; a spec of a component that uses a
\* Worker builds on this module, not on Worker.tla, so its state space holds
\* none of the Worker's internal steps.
\*
\* This module has no config: it is checked through the specs that refine it.
EXTENDS FiniteSets

CONSTANT Jobs

VARIABLES open, pending, ran, stopped

vars == <<open, pending, ran, stopped>>

Init ==
  /\ open = TRUE
  /\ pending = {}
  /\ ran = {}
  /\ stopped = FALSE

Accept(j) ==
  /\ open
  /\ j \notin pending \cup ran
  /\ pending' = pending \cup {j}
  /\ UNCHANGED <<open, ran, stopped>>

Run(j) ==
  /\ j \in pending
  /\ pending' = pending \ {j}
  /\ ran' = ran \cup {j}
  /\ UNCHANGED <<open, stopped>>

Close ==
  /\ open
  /\ open' = FALSE
  /\ UNCHANGED <<pending, ran, stopped>>

Finish ==
  /\ ~open
  /\ ~stopped
  /\ pending = {}
  /\ stopped' = TRUE
  /\ UNCHANGED <<open, pending, ran>>

Next ==
  \/ \E j \in Jobs : Accept(j) \/ Run(j)
  \/ Close
  \/ Finish

Spec == Init /\ [][Next]_vars
====
