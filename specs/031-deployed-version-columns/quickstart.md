# Quickstart: Deployed Version in get and describe

Manual end-to-end verification of feature 031. Assumes a built `shrine` binary,
a running Docker daemon, and a scratch state directory. The agent that delivered
the feature did not run these steps against Docker; CI runs the equivalent
scenarios in `tests/integration/get_test.go` and `describe_test.go`.

## 0. Pre-flight

```bash
go build -o shrine .
state=$(mktemp -d)
./shrine apply teams --path tests/testdata/deploy/team --state-dir "$state"
```

## 1. Deploy and list

```bash
./shrine deploy --path tests/testdata/deploy/resources --state-dir "$state"
./shrine get deployed --state-dir "$state"
# expect a VERSION column after KIND; both rows show  traefik/whoami
./shrine get resources --team shrine-deploy-test --state-dir "$state"
# expect the test-cache row with  traefik/whoami
./shrine get applications --state-dir "$state"
# expect the whoami-res row with  traefik/whoami
```

The listing reads state only; stopping Docker between the deploy and the `get`
does not change the output.

## 2. Describe

```bash
./shrine describe app whoami-res --state-dir "$state"
./shrine describe resource test-cache --state-dir "$state"
# expect, after Kind:
#   Image:        traefik/whoami
#   Pull policy:  Always
```

`traefik/whoami` has no tag, so the derived policy is `Always`.

## 3. A record from the previous release

```bash
f="$state/shrine-deploy-test/deployments.txt"
cat "$f"
# six fields per line
awk '{print $1, $2, $3, $4}' "$f" > "$f.legacy" && mv "$f.legacy" "$f"
./shrine get deployed --state-dir "$state"
# expect  -  in the VERSION column on both rows
./shrine describe app whoami-res --state-dir "$state"
# expect  Image:        -  and  Pull policy:  -
```

## 4. The next deploy fills it in

```bash
./shrine deploy --path tests/testdata/deploy/resources --state-dir "$state"
# nothing is recreated: the config hash is unchanged
./shrine get deployed --state-dir "$state"
# expect  traefik/whoami  on both rows again
cat "$f"
# six fields per line again
```

## 5. Dry run writes nothing

```bash
./shrine deploy --path tests/testdata/deploy/resources --state-dir "$state" --dry-run
# no change to deployments.txt
```

## 6. Clean up

```bash
./shrine teardown shrine-deploy-test --state-dir "$state"
cat "$f"   # empty: the record, with its version, is gone
rm -rf "$state" ./shrine
```
