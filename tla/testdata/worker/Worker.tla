---- MODULE Worker ----
\* A fixture modeled on a Go job queue, worker.go, whose New, Submit, Stop,
\* signal, take and loop it models; the Go code is not part of it. Each action
\* is one step of that code another goroutine can run between: one lock
\* section, one channel send or receive, or one job run. Action names follow
\* the functions they model.
\*
\* Callers, found with rg 'worker\.New\(|\.Submit\(|\.Stop\(':
\*   - one Submit call per job in Jobs, each from its own goroutine; modeled.
\*   - one Stop call, from the owner; modeled. A second Stop call returns at
\*     once on the same done channel and is left out.
\*
\* Left out: what a job does (a job is one atomic step), and the job's own
\* panics. The spec checks that the Worker refines WorkerApi.tla, so specs of
\* the Worker's callers build on WorkerApi instead of this module.
EXTENDS Sequences, FiniteSets

CONSTANTS Jobs, None

VARIABLES
  phase,  \* Worker.phase
  queue,  \* Worker.queue
  wake,   \* a value buffered in Worker.wake
  done,   \* Worker.done is closed
  lpc,    \* where loop is: "take", "run", "close", "wait" or "exited"
  cur,    \* the job loop took and has not finished running, or None
  ran,    \* jobs that have run
  sub,    \* each job's Submit call: "idle", "signal", "accepted", "rejected"
  spc     \* the Stop call: "idle", "signal", "wait" or "returned"

vars == <<phase, queue, wake, done, lpc, cur, ran, sub, spc>>

Accepted == {j \in Jobs : sub[j] \in {"signal", "accepted"}}

Init ==
  /\ phase = "running"
  /\ queue = <<>>
  /\ wake = FALSE
  /\ done = FALSE
  /\ lpc = "take"
  /\ cur = None
  /\ ran = {}
  /\ sub = [j \in Jobs |-> "idle"]
  /\ spc = "idle"

\* Submit's lock section: accept while running, reject after Stop.
Submit(j) ==
  /\ sub[j] = "idle"
  /\ IF phase = "running"
       THEN /\ queue' = Append(queue, j)
            /\ sub' = [sub EXCEPT ![j] = "signal"]
       ELSE /\ sub' = [sub EXCEPT ![j] = "rejected"]
            /\ UNCHANGED queue
  /\ UNCHANGED <<phase, wake, done, lpc, cur, ran, spc>>

\* Submit's signal after the lock is released, then Submit returns true.
SubmitSignal(j) ==
  /\ sub[j] = "signal"
  /\ wake' = TRUE
  /\ sub' = [sub EXCEPT ![j] = "accepted"]
  /\ UNCHANGED <<phase, queue, done, lpc, cur, ran, spc>>

\* Stop's lock section: running becomes draining.
StopMark ==
  /\ spc = "idle"
  /\ phase' = IF phase = "running" THEN "draining" ELSE phase
  /\ spc' = "signal"
  /\ UNCHANGED <<queue, wake, done, lpc, cur, ran, sub>>

StopSignal ==
  /\ spc = "signal"
  /\ wake' = TRUE
  /\ spc' = "wait"
  /\ UNCHANGED <<phase, queue, done, lpc, cur, ran, sub>>

\* Stop returns once done is closed.
StopReturn ==
  /\ spc = "wait"
  /\ done
  /\ spc' = "returned"
  /\ UNCHANGED <<phase, queue, wake, done, lpc, cur, ran, sub>>

\* take's lock section: pop a job, or finish draining when the queue is empty.
Take ==
  /\ lpc = "take"
  /\ IF queue # <<>>
       THEN /\ cur' = Head(queue)
            /\ queue' = Tail(queue)
            /\ lpc' = "run"
            /\ UNCHANGED phase
       ELSE LET p == IF phase = "draining" THEN "stopped" ELSE phase
            IN /\ phase' = p
               /\ lpc' = IF p = "stopped" THEN "close" ELSE "wait"
               /\ UNCHANGED <<cur, queue>>
  /\ UNCHANGED <<wake, done, ran, sub, spc>>

RunJob ==
  /\ lpc = "run"
  /\ ran' = ran \cup {cur}
  /\ cur' = None
  /\ lpc' = "take"
  /\ UNCHANGED <<phase, queue, wake, done, sub, spc>>

CloseDone ==
  /\ lpc = "close"
  /\ done' = TRUE
  /\ lpc' = "exited"
  /\ UNCHANGED <<phase, queue, wake, cur, ran, sub, spc>>

\* loop's receive on wake.
Wait ==
  /\ lpc = "wait"
  /\ wake
  /\ wake' = FALSE
  /\ lpc' = "take"
  /\ UNCHANGED <<phase, queue, done, cur, ran, sub, spc>>

Loop == Take \/ RunJob \/ CloseDone \/ Wait

\* Every call has returned and the loop has exited.
Done ==
  /\ lpc = "exited"
  /\ spc = "returned"
  /\ \A j \in Jobs : sub[j] \in {"accepted", "rejected"}
  /\ UNCHANGED vars

Next ==
  \/ \E j \in Jobs : Submit(j) \/ SubmitSignal(j)
  \/ StopMark \/ StopSignal \/ StopReturn
  \/ Loop
  \/ Done

\* The loop goroutine and every call already in progress keep running. Nothing
\* forces a caller to call Submit or Stop.
Spec ==
  /\ Init /\ [][Next]_vars
  /\ WF_vars(Loop)
  /\ \A j \in Jobs : WF_vars(SubmitSignal(j))
  /\ WF_vars(StopSignal)
  /\ WF_vars(StopReturn)

TypeOK ==
  /\ phase \in {"running", "draining", "stopped"}
  /\ lpc \in {"take", "run", "close", "wait", "exited"}
  /\ cur \in Jobs \cup {None}
  /\ ran \subseteq Jobs
  /\ spc \in {"idle", "signal", "wait", "returned"}

\* Stop returns only after every accepted job has run.
AcceptedRunBeforeStopReturns == spc = "returned" => Accepted \subseteq ran

\* Submit rejects only after Stop has been called.
RejectOnlyAfterStop == \A j \in Jobs : sub[j] = "rejected" => phase # "running"

\* A Stop call returns.
StopReturns == spc # "idle" ~> spc = "returned"

\* An accepted job runs, whether or not Stop is ever called.
AcceptedRuns == \A j \in Jobs : j \in Accepted ~> j \in ran

API == INSTANCE WorkerApi WITH
  open <- phase = "running",
  pending <- Accepted \ ran,
  stopped <- done

\* Every step of the Worker is a step of WorkerApi, or leaves it unchanged.
Refines == API!Spec
====
