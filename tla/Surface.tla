---- MODULE Surface ----
\* Rules every screen keeps, for every person who can reach it. A spec
\* instances this module with its People, the actions each person is
\* Shown, and the actions the enforcing component Allows.
CONSTANTS People, Actions, Shown(_), Allowed(_, _), Next(_)

\* Every action on screen succeeds for the person who sees it.
EveryShownActionWorks ==
  \A p \in People : \A a \in Shown(p) : Allowed(p, a)

\* Every person has something to do, or the spec says why not.
NobodyIsStuck ==
  \A p \in People : Shown(p) # {} \/ Next(p) # ""
====
