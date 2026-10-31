---- MODULE Counter ----
\* A counter that forgets EXTENDS Naturals, so its < and + are undefined.
VARIABLE x
Init == x = 0
Next == x < 2 /\ x' = x + 1
Spec == Init /\ [][Next]_x
====
