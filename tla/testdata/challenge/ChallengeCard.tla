---- MODULE ChallengeCard ----
\* The Discord challenge from the Hyperline lobby bot plan. Mira challenges
\* Jun; Pax watches the channel. A Discord message shows the same buttons to
\* everyone who can see it, so a button belongs to a message, not a viewer.
EXTENDS FiniteSets

CONSTANT Design  \* "shared": Accept and Decline on the channel card.
                 \* "private": each button in a message only its presser sees.

People == {"mira", "jun", "pax"}
Challenger == "mira"
Target == "jun"

VARIABLES challenge, refused
vars == <<challenge, refused>>

\* Messages and who sees each one.
Messages ==
  IF Design = "shared" THEN {"card"} ELSE {"card", "dm", "reply"}
Sees(m) ==
  CASE m = "card"  -> People
    [] m = "dm"    -> {Target}
    [] m = "reply" -> {Challenger}

\* Buttons each message carries in each challenge state.
Buttons(m) ==
  IF challenge # "open" THEN {}
  ELSE IF Design = "shared" THEN {"accept", "decline"}
  ELSE CASE m = "card"  -> {}
         [] m = "dm"    -> {"accept", "decline"}
         [] m = "reply" -> {"withdraw"}

Shown(p) == UNION {Buttons(m) : m \in {x \in Messages : p \in Sees(x)}}

\* The rule the backend enforces.
Allowed(p, b) ==
  \/ p = Target /\ b \in {"accept", "decline"}
  \/ p = Challenger /\ b = "withdraw"

Result(b) ==
  CASE b = "accept" -> "accepted" [] b = "decline" -> "declined"
    [] b = "withdraw" -> "withdrawn"

Init == challenge = "open" /\ refused = {}

\* A person presses a button they can see.
Press(p, b) ==
  /\ b \in Shown(p)
  /\ IF Allowed(p, b)
       THEN challenge' = Result(b) /\ UNCHANGED refused
       ELSE refused' = refused \cup {<<p, b>>} /\ UNCHANGED challenge

Expire == challenge = "open" /\ challenge' = "expired" /\ UNCHANGED refused
Done == challenge # "open" /\ UNCHANGED vars

Next == (\E p \in People, b \in {"accept", "decline", "withdraw"} : Press(p, b))
        \/ Expire \/ Done

\* Every button a person sees works for them.
EveryButtonWorks == refused = {}
\* The challenger can take back an open challenge.
ChallengerCanWithdraw == challenge = "open" => "withdraw" \in Shown(Challenger)
\* Nobody but Jun sees Accept.
OnlyTargetSeesAccept == \A p \in People \ {Target} : "accept" \notin Shown(p)
====
