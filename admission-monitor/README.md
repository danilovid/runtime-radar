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

## Web interface

Sources are managed on the **Admission** page of the web interface (`radar-ui`), which is the
admission counterpart of the runtime sources page: a source is enabled with a checkbox, its action
and severity are picked from selects, and expert mode allows editing the manifest or adding a source
of your own.

Response rules of type `admission` are created on the common rules page: the rule form asks for the
type and then shows the whitelist that belongs to it. A runtime rule whitelists WASM detectors picked
from a tree plus process binaries; an admission rule whitelists Kyverno policies typed in as
`<policy name>/<rule name>`. Both end up in the `threats` field of the rule, `binaries` stays empty
for an admission rule.

To receive notifications, the notification target must be created with the `Admission` event type:
a target is bound to one type, and a rule only sees targets of its own type.

Admission events are listed on the **Events** tab and open into a card with the resource, its
containers, the policies that fired and the rules that made the finding an incident. They are read
from `history-api` over `/api/v1/admission-event`.

The public API (`public-api`) does not expose admission events yet: it proxies runtime events only.

## Two sources of findings

Policy reports only describe resources that exist in the cluster. A request denied by Kyverno never
becomes a resource, so it produces no report, see https://kyverno.io/docs/guides/reports/. This is
why admission-monitor watches two things at once:

| Source | Covers | Event |
|---|---|---|
| `PolicyReport` / `ClusterPolicyReport` | audited violations and background scans | `blocked: false` |
| Kubernetes events | requests denied at admission | `blocked: true` |

The two never overlap. Kyverno emits an event for every result of the webhook, and the action tells
them apart: a denied request is `Resource Blocked`, an audited violation is `Resource Passed` and is
skipped because a report already covers it.

An event is reported on the policy, and the resource that was denied is its related object. The name
of the failed rule appears only inside the message, so it is parsed out of it: that keeps the threat
identifier (`<policy name>/<rule name>`) the same for a blocked and for an audited finding, so one
whitelist entry works for both. A blocked resource never exists, so such an event carries no
containers and rules scoped by image or container do not match it.

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
