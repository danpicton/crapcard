# Preserve card scheduling across note edits

A note can produce several independently scheduled cards. We identify those
cards by their stable template and preserve the scheduling and review history
of every template that survives a note edit. Removing a template removes its
card and history. This lets users improve wording without losing learning
progress, while allowing a note's type and number of cards to change; the
alternative of regenerating every card on save would reset that progress.
