# Contract: Operator Output

**Feature**: 031-deployed-version-columns | Binding: design section 4.7, T1-03 to T1-05

## `shrine get deployed` / `get applications` / `get resources`

Row format `%-20s %-30s %-15s %-40s %-15s`; the separator is as long as the header.

```text
TEAM                 NAME                           KIND            VERSION                                  CONTAINER ID
----------------------------------------------------------------------------------------------------------------------------
shrine-deploy-test   test-cache                     Resource        traefik/whoami                           3f2a9c1b7d4e
shrine-deploy-test   whoami-res                     Application     traefik/whoami                           9b8c7d6e5f4a
```

- VERSION is the fifth column, after KIND and before CONTAINER ID.
- VERSION prints `Deployment.Image` verbatim (`reg:lab/hello-api:1.2.0` stays
  `reg:lab/hello-api:1.2.0`), or `-` when the record has no image.
- TEAM, NAME, KIND, and CONTAINER ID keep their header, order, and values.
- Column widths and the separator length are not a contract.
- The same table serves all three commands, with and without `--team`.
- The "No deployments found." / "No applications deployed." / "No resources
  deployed." empty-state lines are unchanged.

## `shrine describe app <name>` / `describe resource <name>`

```text
Name:         whoami
Team:         shrine-deploy-test
Kind:         Application
Image:        traefik/whoami
Pull policy:  Always
Container ID: 9b8c7d6e5f4a1234…
Config Hash:  3a7b1c9d2e4f5a6b...
```

- `Image:` and `Pull policy:` follow `Kind:` and precede `Container ID:`.
- Labels are padded to the existing fourteen-character column.
- Each prints `-` when the record has no value:

```text
Image:        -
Pull policy:  -
```

- Every existing line keeps its label, order, and value. Lookup, `--team`
  disambiguation, and the "not found" / "ambiguous" errors are unchanged.
