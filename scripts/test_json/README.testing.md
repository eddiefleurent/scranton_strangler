This directory contains Go unit tests built with the standard library "testing" package.
If the repository conventionally uses additional testing libraries (e.g., testify), you can
adapt imports and assertions accordingly without changing test intent.

Coverage focus:
- JSON marshaling of models.Position omits StateMachine field (json:"-")
- Round-trip state preservation across marshal/unmarshal
- Lazy initialization after unmarshal via GetManagementPhase()
- Invalid state transition scenarios produce errors

To run:
  go test ./...