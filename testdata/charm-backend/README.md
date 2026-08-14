# Charm backend parity fixtures

These inputs are intentionally portable. `@ROOT@` is replaced with a temporary
directory by each implementation, and the manifest fixes video modification times
because Git does not preserve them.

`library-basic.expected.json` is the canonical, explicitly ordered snapshot shared
by the bridge characterization tests and future Go library tests.
