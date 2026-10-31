---- MODULE MapPicker ----
\* The lobby Map dropdown from the October 15 state sheets dive. A map
\* with no setup can be picked only by a leader who may place one.
CONSTANT Design  \* "all": lists every approved map. "rule": applies the rule.

People == {"modder", "leader"}
Maps == {"setup", "nosetup"}
MayPlace(p) == p = "modder"
Allowed(p, m) == m = "setup" \/ MayPlace(p)
Shown(p) == IF Design = "all" THEN Maps ELSE {m \in Maps : Allowed(p, m)}
Hint(p) == ""

VARIABLE picked
Init == picked = "setup"
Pick(p, m) == m \in Shown(p) /\ picked' = m
Spec == Init /\ [][\E p \in People, m \in Maps : Pick(p, m)]_picked

S == INSTANCE Surface WITH Actions <- Maps, Next <- Hint
EveryShownActionWorks == S!EveryShownActionWorks
NobodyIsStuck == S!NobodyIsStuck
====
