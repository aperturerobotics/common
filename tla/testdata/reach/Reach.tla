---- MODULE Reach ----
\* A release ships after two iterations, or never when it waits on a stalled
\* scope. Each config claims the ship is reachable.
EXTENDS Naturals
CONSTANT Stalled
VARIABLE left
Init == left = 2
Next == ~Stalled /\ left > 0 /\ left' = left - 1
Shipped == left = 0
====
