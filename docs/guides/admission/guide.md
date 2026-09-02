# Admission control: sources, policies and detectors

## What admission control is

Runtime Radar watches two moments in the life of a workload, and they are
independent of each other.

**Runtime.** Tetragon records what processes do inside workloads that are
already running. WASM detectors flag threats in those events. An event here is
something that happened.

**Admission.** Kyverno checks a resource as it is submitted to the cluster,
before anything runs. A finding here is about a manifest, not about a process:
it proves that what was asked for breaks a policy, not that the workload did
anything. A finding can concern any kind of resource, not only pods —
ClusterRoleBindings, Deployments and ReplicaSets all pass through admission.

The two halves are configured separately, and a rule written for one says
nothing about the other.

## Sources

A source is one Kyverno policy Runtime Radar keeps applied in the cluster. Each
source has:

* a **key** — a stable identifier such as `privileged-containers`, used by the
  API and never shown to the user;
* a **name** and a **description** — what the operator sees;
* an **action** — `audit` records the violation and lets the request through,
  `block` makes Kyverno deny it;
* a **severity** — `low`, `medium`, `high` or `critical`, carried into the
  findings the source produces;
* a **manifest** — the Kyverno policy itself, a `ValidatingPolicy` or an
  `ImageValidatingPolicy` of `policies.kyverno.io/v1`.

Runtime Radar owns the policies it applies: it labels them
`app.kubernetes.io/managed-by=admission-monitor`, and on every configuration
change it removes the ones it manages and applies the enabled set again. A
policy edited directly in the cluster is therefore overwritten. Edit the source,
not the `ValidatingPolicy` object.

The `spec.validationActions` of the manifest is overwritten from the source's
action, so a manifest that says `Deny` does not block anything until the source
itself is switched to the blocking mode.

## Managing sources in the interface

Open the **Admission** section, tab **Sources**. Each source has a checkbox that
applies or removes it, a list to choose between audit and blocking, and a list
to choose the severity of its findings. Changes take effect without a restart.

The **Events** tab lists the findings: which resource was checked, which
policies fired and whether the request was allowed or refused.

## Creating and editing a source from the chat

The built-in assistant can add a source and switch existing ones. Describe what
you want checked, in your own words, and it will propose the call. Nothing is
applied until you press the button confirming it.

To add a source the assistant needs four things, and it will ask for whatever
you have not given:

* the identifier — lowercase with dashes, for example `require-run-as-non-root`;
* the name shown in the interface;
* the description of what the source detects;
* the Kyverno manifest. You can paste one you already have, or describe the rule
  and let the assistant write it; either way, read it before confirming.

A source added this way is **always created switched off**. Nothing changes in
the cluster until you enable it, which is a second, separate step — deliberately,
so that a policy nobody has read cannot start refusing requests.

To change an existing source, ask the assistant to enable or disable it, to
switch it between audit and blocking, or to change the severity of its findings.
It reads the current set with `list_admission_sources` first, so you can ask
what exists before changing anything.

A worked example of what to say:

> Add a source that refuses pods without `runAsNonRoot`. Call it "Containers
> running as root", severity high. Leave it in audit for now.

The assistant writes the manifest, shows it to you, and creates the source
switched off in the audit mode. You read the manifest, enable the source, watch
the findings for a while, and only then switch it to blocking.

## At your own risk

**A source in the blocking mode refuses requests to the cluster.** That is the
whole point of it, and it is also how a deployment stops going through, a
CI pipeline starts failing, and — if the policy matches more than you meant —
how a cluster stops being able to schedule anything at all.

This holds whoever wrote the policy, and it holds especially for a policy
written from a description in a chat. The assistant is a language model: it
produces a plausible manifest, not a verified one. It does not know your
workloads, it cannot try the policy out before proposing it, and a rule that
looks right can match far wider than you intended. Nobody reviews what it wrote
except you.

So, before you enable a source, and before you switch one to blocking:

1. **Read the manifest.** Every match block, every exclusion. If you do not
   understand a line of it, do not enable it.
2. **Start in audit.** Let it record for long enough to cover a normal working
   day, a deploy and a rollback.
3. **Look at what it caught.** The Events tab shows every resource the source
   would have refused. If there is anything there you did not expect, the policy
   is wrong, not the workload.
4. **Only then switch to blocking**, and watch the first deployments through it.

Runtime Radar excludes its own namespace from Kyverno's checks, so a policy
cannot lock the product itself out. It does **not** protect the rest of your
cluster from a policy you enabled.

The responsibility for what a policy does in your cluster is yours. The
assistant helps you write it; it does not answer for it.

## Response rules for admission findings

A finding on its own is a record. What happens next is decided by response
rules, in the **Rules** section, where a rule of the admission type matches
findings by policy, namespace, image or severity and raises a notification.

The assistant can create and delete response rules too, with the same
confirmation step, and the same warning applies: a rule that blocks stops
workloads.

Note that a blocked admission request has no containers — the pod was never
created — so a rule scoped by image or by container name will not match findings
from a source in the blocking mode. Scope such rules by policy or namespace.

## Detectors

Detectors belong to the runtime half. A detector is a WebAssembly module that
examines the events Tetragon produced and decides whether they contain a threat:
`CS_RT_SUSP_FILE_READ` for a read of a sensitive file, `CS_RT_SSH_KEY_MODIFY`
for a change to `authorized_keys`, and so on.

What you can do in the interface and from the chat:

* **See what is installed.** Ask the assistant which detectors exist and what
  each one looks for; it reads them with `list_detectors`.
* **Choose what is recorded.** The runtime monitoring settings decide which
  events reach the detectors at all — an allow list of namespaces, pods and
  binaries — and whether every event is stored or only the ones with threats.
  A narrow allow list is the usual reason a threat you expected never appeared.
* **Choose what happens on a threat.** Response rules of the runtime type match
  detector identifiers and decide whether to notify or to kill the pod.

**A new detector cannot be created from the chat, and this is not a limitation
of the assistant.** A detector is a program in Go, compiled to WebAssembly with
TinyGo and shipped with the product; writing one means writing code, building it
and installing the module. The assistant has no tool that does this, and it will
tell you so rather than pretend. If you need a detector that does not exist,
see the developer guide in `docs/guides/detectors/guide.md`.

What often replaces a new detector: an existing detector plus a response rule
with a narrower scope, or an admission source that refuses the thing at
submission instead of catching it at runtime. Ask the assistant which of the
three fits what you are trying to catch — that question it can answer.
