# admission-monitor
Admission Monitor manages Kyverno policies and turns policy reports into Runtime Radar events.

## How it fits the pipeline

It is the admission counterpart of `runtime-monitor`: where `runtime-monitor` configures Tetragon
and reads its event stream, `admission-monitor` configures Kyverno and reads its policy reports.

```
Kyverno --(PolicyReport)--> admission-monitor --> policy-enforcer (admission rules)
                                     |--> rabbitmq(admission_events) --> history-api
                                     `--> notifier
```

Kyverno is the only component in the admission path, so it is Kyverno that allows or denies a request.
`policy-enforcer` rules of type `admission` decide whether a finding is an incident and who is notified
about it, exactly as they do for runtime events.

## Sources

A source is a Kyverno policy (`ValidatingPolicy` or `ImageValidatingPolicy`) stored in the config
together with a name, a description, a severity and an action. Defaults are embedded in
`pkg/model/policy` and are all disabled out of the box. The config is exposed over
`/api/v1/config/admission-monitor` the same way runtime sources are.

Policies applied by admission-monitor are labeled `app.kubernetes.io/managed-by=admission-monitor`.
Policies without that label are never modified, so Kyverno can be shared with policies managed
outside of Runtime Radar.

`spec.validationActions` of a manifest is always overwritten from the source's action, so switching
a source between audit and enforce never requires editing YAML in expert mode:

| Action    | validationActions | Kyverno behaviour                             |
|-----------|-------------------|-----------------------------------------------|
| `AUDIT`   | `[Audit]`         | request is allowed, violation lands in a report |
| `ENFORCE` | `[Deny]`          | request is denied                              |

## Known limitation

Policy reports only describe resources that exist in the cluster. A request denied by Kyverno never
becomes a resource, so it produces no report: Kyverno reports blocked requests via Kubernetes events
and metrics instead, see https://kyverno.io/docs/guides/reports/. This means a source in the enforce
mode blocks the request but produces no Runtime Radar event.

TODO: watch Kubernetes events with `reason=PolicyViolation` produced by the `kyverno-admission`
component to also record blocked requests.

## Deployment notes

Kyverno and Runtime Radar must exclude each other:

* Runtime Radar's namespace is added to Kyverno's `resourceFilters` by the umbrella chart, otherwise
  a simultaneous restart of both leaves the cluster unable to schedule either of them.
* Reports of Kyverno's own namespace are skipped by admission-monitor (`KYVERNO_NAMESPACE`).

Kyverno's webhooks default to `failurePolicy: Fail`. Runtime Radar does not change that default;
decide it deliberately before enabling any source in the enforce mode.

Run a single replica: policies are applied and reports are consumed by every instance, so more than
one replica means duplicated events.

The component is not part of the root `docker-compose.yaml`: unlike Tetragon, which runs as a
container next to runtime-monitor, Kyverno needs a real Kubernetes API server, so admission-monitor
can only be run in a cluster.
