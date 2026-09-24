# Tests and fixtures

No test needs hardware. A test builds a fixture tree in `t.TempDir()`, writes the
sysfs or `/proc` files it wants, points the package's path variable at the tree and
restores the original in `t.Cleanup`. Copy the helper that package already has
instead of inventing a second pattern; `test-index.md` lists every helper with its
line, and `go test ./...` keeps that list current.

| Package | Fixture helpers | Variables to aim at a fixture |
|---------|-----------------|-------------------------------|
| `cpuinfo` | `writeHWMonFixture` | `sysDir` |
| `mainboard` | `writeSysFixture`, `boardFixture` | `sysDir` |
| `storage` | `writeSysFixture`, `writeMountsFixture`, `stubUDisks`, `stubFilesystemUsage` | `sysDir`, `mountsPath`, `filesystemUsage`, `smartProvider` |
| `memory` | `writeFixture`, `writeZramDevice` | `meminfoPath`, `psiPath`, `zramPath` |
| `battery` | `writeSupplies`, `energyBattery` | `sysDir` |
| `amd` / `intel` | fixtures built inside their own `_test.go` | `sysDir` |
| `shared` | `writeCard` | the `sysDir` argument the helper takes |
| `main` | `cpuValues`, `boardViewFor`, `diskSize` | pure functions only — no sysfs |

`nvidia` is tested as pure parsing instead: the `gpuQueryFields` column order,
`parseCSVFields` with a comma inside a name (columns must stay aligned), and
`parseGPURecord` omitting the fields the driver reports as `[N/A]`.

## Patterns worth copying

- **Both directions.** The value that should appear, with its unit *and* its type,
  and the absent case producing **no key** — not `""`, not `0`. A fixture that omits
  a file must be asserted on: that is the invariant the whole app is built around.
- **`shared.ResetProbeCache()`** in any test that rewrites a fixture in place,
  because identity probes are memoized for the process lifetime. The cache key
  includes the sysfs root, which is why a test can point a collector at its own tree
  without reading another test's entry.
- **Stub the bus, never the host.** `stubUDisks` replaces `smartProvider` so a
  storage test never reaches the real `udisksd`; `stubFilesystemUsage` records the
  mount points it was asked about, which is how per-drive aggregation is asserted.
- **Names read as sentences about the behaviour**, not the function:
  `TestCPUIntelCoretempTemperatures`, `TestParseCpuInfoMalformedDoesNotPanic`,
  `TestZramUnconfiguredDevicesAreSkipped`,
  `TestAtaSmartInfoKeepsAbsentBadSectorsAbsent`. A regression test is named for the
  failure it pins and listed under `Issues addressed` in `CODE_REVIEW.md`.
- **Caps are asserted too** — `TestCPUTccdSensorsAreCapped`, `TestFanCountIsCapped` —
  because an unbounded card is the failure the constant prevents.
- **The root package guards the key contract.**
  `Test*TemplatesMatchCollectorKeyContract` in `main_test.go` catches a template
  whose key no collector produces, or a collector key no template renders. Extend it
  with any new row.

## Running them

```bash
make test                      # go test ./...
go test ./... -count=1 -race   # what CI runs; -race matters for the caches
go test ./storage -run TestAddSmart -v
```
