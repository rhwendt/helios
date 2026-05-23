# Helios Implementation - Agent Team Prompt

## Project Overview

Implement **Helios**, a CNCF-native, Kubernetes-first network observability platform. Full specification is at `/home/rwendt/git/helios/specs/001-helios-platform/`.

**Stack**: gnmic (gNMI) + Prometheus/Thanos (metrics) + goflow2/Kafka/ClickHouse (flows) + custom Go services + Helm umbrella chart + ArgoCD
**Target Hardware**: 3x Miniforum MS-01 (x86) + 5x Raspberry Pi 5 8GB (ARM) — multi-arch required
**Scale**: 4 tiers (Small <50, Medium 50-500, Large 500-2000, XL 2000-8000+ devices)
**Languages**: Go 1.22+ (all 3 custom services), YAML/JSON (Helm, dashboards, alerts)
**Working directory**: `/home/rwendt/git/helios/`

---

## Specification Files (MUST READ)

All agents MUST read these files before starting any work. These are the **authoritative source of truth** — do not deviate from them.

| File | What It Contains | Read Priority |
|------|-----------------|---------------|
| `specs/001-helios-platform/spec.md` | User stories (US1-US6), acceptance scenarios (Given/When/Then), functional requirements (FR-001 through FR-032), key entities, success criteria (SC-001 through SC-012), edge cases | **READ FIRST** |
| `specs/001-helios-platform/tasks.md` | **150 tasks (T001-T150)** across 9 phases with exact file paths, parallel markers [P], user story labels [US1-US6], and dependency order | **READ SECOND** — this is your task backlog |
| `specs/001-helios-platform/plan.md` | Project structure (repo layout), technical context, constitution check, complexity tracking, implementation strategy, parallel team strategy | **READ THIRD** |
| `specs/001-helios-platform/data-model.md` | Entity definitions (Device, Interface, Metric, Flow Record, Flow Aggregates, Runbook, RunbookExecution, Target ConfigMap), ClickHouse schemas, label taxonomy, entity relationships diagram | Reference during implementation |
| `specs/001-helios-platform/research.md` | 10 research decisions (R-001 through R-010) with rationale and rejected alternatives: gnmic clustering, Prometheus sharding, ClickHouse pattern, Kafka pipeline, Flow Enricher design, storage topologies, kubebuilder operator, Target Generator, multi-arch builds, dashboard provisioning | Reference when making technical choices |
| `specs/001-helios-platform/quickstart.md` | End-user quickstart guide — the target UX for the finished product | Reference for UX validation |
| `specs/001-helios-platform/checklists/requirements.md` | Requirements traceability matrix: FR → US mapping, constitution compliance, success criteria coverage | Reference for completeness checks |
| `specs/001-helios-platform/contracts/runbook-crd.yaml` | Runbook CRD OpenAPI v3 schema — exact field definitions, enums, validation rules | Implementation contract for US5 |
| `specs/001-helios-platform/contracts/runbookexecution-crd.yaml` | RunbookExecution CRD schema — state machine phases, step tracking, audit fields | Implementation contract for US5 |
| `specs/001-helios-platform/contracts/flow.proto` | Protobuf definition for EnrichedFlow message — canonical format between Flow Enricher and ClickHouse | Implementation contract for US3 |
| `specs/001-helios-platform/contracts/target-generator-output.yaml` | Target Generator output format — gnmic targets YAML, SNMP/blackbox file_sd JSON, Prometheus metrics | Implementation contract for US4 |

---

## Team Structure

### Team Roles

**project-manager** (Coordinates all work)
- Owns the task list and phase progression
- Tasks are pre-defined in `specs/001-helios-platform/tasks.md` (T001-T150) — create team tasks from this backlog
- Assigns tasks to developers respecting dependency order and [P] parallel markers
- Tracks blockers, manages dependencies between phases
- Ensures phases complete in order with acceptance criteria met (see spec.md user stories)
- **CRITICAL**: A task is NOT complete until the auditor has reviewed and approved the code. Route all completed developer work to the auditor before marking any task as done.
- Reports progress and escalates blockers
- Agent type: `general-purpose`

**developer-1** through **developer-3** (Implementation)
- Write Helm charts, Kubernetes manifests, Go services per the spec
- Follow the exact file paths specified in `tasks.md` — every task has an explicit output path
- Follow the repo structure defined in `plan.md` (Section: "Source Code (repository root)")
- All Go code uses Go 1.22+, `slog` for logging, context propagation, table-driven tests
- All Helm charts use sub-chart pattern under `helm/helios/charts/helios-*/`
- **Can directly consult the technical-advisor** for domain questions (gNMI protocol details, Thanos internals, ClickHouse schema design, kubebuilder patterns, etc.) without going through the PM. Message the advisor directly when you hit a technical question — don't block waiting for PM routing.
- After completing code, notify the project-manager for auditor review. Do NOT consider your work done until the auditor approves it.
- Address all BLOCKING auditor findings before the task can be marked complete
- Agent type: `developer`

**auditor** (Security, Code Quality, Issues - GATEKEEPER)
- **All code must pass auditor review before a task can be marked complete**
- Reviews all code produced by developers — this is a mandatory gate, not optional
- Validates against:
  - **spec.md**: Do acceptance scenarios pass? Are functional requirements met?
  - **data-model.md**: Do schemas, labels, and entity structures match?
  - **contracts/**: Do CRDs, protobuf, and target output formats match the contracts exactly?
  - **research.md**: Are the chosen technologies and patterns consistent with research decisions?
- Checks for: security vulnerabilities (RBAC gaps, missing NetworkPolicies, exposed secrets, no ESO), code smells, K8s anti-patterns, Helm best practices, Go idioms
- Runs `helm lint`, `golangci-lint`, validates manifests where possible
- Returns findings to developers with severity levels:
  - **BLOCKING**: Must fix before task completion
  - **WARNING**: Should fix, can be tracked as follow-up task
  - **INFO**: Suggestions for improvement, non-blocking
- Only after auditor gives explicit approval can the PM mark a task as completed
- Agent type: `auditor`

**tester** (Tests & Coverage)
- Owns the test-first tasks (marked ⚠️ in `tasks.md`): writes helm unittest YAML tests, Go unit tests, and integration test scaffolding
- Writes tests BEFORE developers implement — tests must FAIL initially, confirming they test something real
- Validates test coverage meets SC-009 (>70% for Go services) using `go test -cover`
- Runs `helm unittest` on all chart test files and reports results
- Creates integration test fixtures (Containerlab topologies, kind cluster configs) for Phase 9
- Works in parallel with developers: tester writes tests for a phase while developers implement the previous phase
- Reports coverage gaps to PM for follow-up task creation
- Agent type: `tester`

**technical-advisor** (Domain Expertise)
- Answers technical questions from developers, testers, and auditors directly — any team member can message the advisor without going through the PM
- Domain expertise: gNMI protocol, Prometheus/Thanos internals, ClickHouse schema design, Kafka/Strimzi configuration, kubebuilder operator patterns, Helm chart structure, ArgoCD patterns, gnmic clustering, protobuf encoding
- Reads spec files and provides clarification on ambiguous specifications
- Consults `research.md` decisions when team members are unsure about technology choices
- Can be consulted by the auditor for domain-specific security or correctness concerns
- Agent type: `general-purpose`

### Task Completion Workflow

```
Tester writes tests (FAIL expected)
        ↓
Developer implements code (tests PASS)
        ↓
Developer notifies PM → PM assigns auditor review
        ↓
Auditor reviews code against spec + contracts
        ↓
    ┌─── BLOCKING findings? ───┐
    │ YES                      │ NO
    ↓                          ↓
Developer fixes issues    Auditor approves
    ↓                          ↓
Re-submit to auditor      Tester verifies coverage
                                ↓
                          PM marks task complete
```

**No exceptions**: Every task that produces code must go through auditor review. The PM must enforce this gate.

### Communication Rules

- **Developers → technical-advisor**: Direct messages allowed for technical questions. Don't wait for PM routing.
- **Tester → technical-advisor**: Direct messages allowed for test design questions (e.g., "what edge cases should I cover for gnmic reconnection?").
- **Auditor → technical-advisor**: Direct messages allowed for domain-specific correctness validation.
- **All → PM**: Status updates, blockers, and task completion notifications go through PM.

---

## Repository Structure (from plan.md)

```
helios/
├── helm/
│   └── helios/                          # Umbrella chart
│       ├── Chart.yaml                   # Dependencies: kube-prometheus-stack, thanos, minio
│       ├── values.yaml                  # Global defaults (Medium scale)
│       ├── values-small.yaml
│       ├── values-medium.yaml
│       ├── values-large.yaml
│       ├── values-xl.yaml
│       ├── charts/
│       │   ├── helios-integration/      # NetBox sync, target generation
│       │   ├── helios-collection/       # gnmic, SNMP, blackbox exporters
│       │   ├── helios-storage/          # Prometheus, Thanos, MinIO
│       │   ├── helios-visualization/    # Grafana, Alertmanager
│       │   ├── helios-automation/       # Runbook Operator
│       │   └── helios-flows/            # goflow2, Kafka, Flow Enricher, ClickHouse
│       └── templates/
│           ├── namespaces.yaml
│           └── networkpolicies-global.yaml
├── services/
│   ├── flow-enricher/                   # Custom Go service
│   ├── runbook-operator/                # Custom K8s operator (kubebuilder)
│   └── target-generator/               # NetBox sync CronJob (Go)
├── config/
│   ├── snmp/modules/                    # SNMP exporter modules
│   ├── gnmic/subscriptions/             # gNMI subscription definitions
│   ├── dashboards/metrics/              # 8 core metrics dashboards (JSON)
│   ├── dashboards/flows/                # 6 flow dashboards (JSON)
│   ├── alerts/rules/                    # PrometheusRule YAML files
│   └── runbooks/samples/                # 10 sample runbook YAMLs
├── clickhouse/migrations/               # ClickHouse schema SQL
├── proto/flow.proto                     # Canonical protobuf
├── deploy/argocd/applicationset.yaml    # ArgoCD
├── .github/workflows/                   # CI pipelines
├── .golangci.yml
├── Makefile
└── README.md
```

---

## Implementation Phases (from tasks.md)

The task backlog in `tasks.md` defines **9 phases** with **150 tasks**. Here is the summary — the PM should read `tasks.md` for the full task list with exact file paths.

### Phase 1: Setup (T001-T010)
Repo structure, Helm umbrella chart skeleton, CI pipeline scaffolding, Makefile, golangci config.
- All tasks can run in parallel after T001-T002.

### Phase 2: Foundational — BLOCKS ALL USER STORIES (T011-T019)
Core storage infrastructure: Prometheus Operator CR with sharding, Thanos stack (Query, Store, Compactor), MinIO, global NetworkPolicies, RBAC roles.
- **No user story work can begin until Phase 2 is complete.**

### Phase 3: US1 — Metrics Collection & Storage (T020-T035) — MVP
gnmic StatefulSet with K8s clustering, SNMP exporter, blackbox exporter, HPAs, ServiceMonitors, SNMP modules, gNMI subscriptions, collection NetworkPolicy, Thanos Ruler.
- Tests first (T020-T023), then implementation (T024-T035).

### Phase 4: US2 — Visualization & Alerting (T036-T052)
Grafana with provisioned datasources, 8 metrics dashboards (JSON), Alertmanager tiered routing, PrometheusRule default alerts.
- Tests first (T036-T038), then implementation (T039-T052).

### Phase 5: US3 — Flow Collection & Analysis (T053-T086)
goflow2, Strimzi Kafka, Flow Enricher (Go service), ClickHouse (Altinity Operator), 4 ClickHouse migrations, 6 flow dashboards, protobuf definition.
- Tests first (T053-T056), then implementation (T057-T086).
- Largest phase — 34 tasks.

### Phase 6: US4 — Device Discovery & Integration (T087-T102)
Target Generator (Go CronJob), NetBox API client, ConfigMap generators (gnmic, SNMP, blackbox, Prometheus SD), atomic ConfigMap updates, CronJob Helm template.
- Tests first (T087-T089), then implementation (T090-T102).

### Phase 7: US5 — Automated Remediation (T103-T130)
Runbook Operator (kubebuilder), CRD types per `contracts/runbook-crd.yaml` and `contracts/runbookexecution-crd.yaml`, controller state machine, gNMI client wrapper, executor Job, template engine, approval handler, audit logger, 10 sample runbooks.
- Tests first (T103-T106), then implementation (T107-T130).

### Phase 8: US6 — Multi-Datacenter Federation (T131-T141)
Storage topology selection (`global.storageTopology`), conditional Thanos/MinIO configs, site replication, cross-DC Thanos Query federation, Compactor singleton, ArgoCD ApplicationSet.
- Tests first (T131-T132), then implementation (T133-T141).

### Phase 9: Polish & Cross-Cutting (T142-T150)
Integration test CI, helm lint, golangci-lint, >70% Go test coverage, multi-arch image verification, quickstart validation, dashboard verification, alert verification.

---

## Phase Dependencies (from tasks.md)

```
Phase 1 (Setup) → no deps, start immediately
Phase 2 (Foundational) → depends on Phase 1, BLOCKS everything
Phase 3 (US1) → depends on Phase 2
Phase 4 (US2) → depends on Phase 2 (can parallel chart work with US1)
Phase 5 (US3) → depends on Phase 2 (Go service dev can parallel with US1/US2)
Phase 6 (US4) → depends on Phase 2 (Go service dev can parallel)
Phase 7 (US5) → depends on Phase 2 (Go service dev can parallel)
Phase 8 (US6) → depends on Phase 2 + Phase 3 (needs working single-DC Thanos)
Phase 9 (Polish) → depends on all phases
```

### Parallel Team Strategy (from plan.md)

After Phase 2, the team works in parallel:
- **Tester**: Writes tests for the NEXT phase while developers implement the current phase. Starts by writing US1 tests (T020-T023) during Phase 2, then US2/US3 tests while devs work on US1, etc.
- **Developer 1**: US1 (Helm charts for metrics collection)
- **Developer 2**: US3 (Flow Enricher Go service) + US5 (Runbook Operator Go service)
- **Developer 3**: US2 (Dashboards + Alerts) + US4 (Target Generator Go service)
- **Auditor**: Reviews completed work from any developer as it comes in
- **Technical Advisor**: Available on-demand for any team member

US6 and Phase 9 are done after earlier stories complete.

---

## Auditor Review Criteria

### Security (check against spec.md FR-028, FR-029)
- **RBAC**: 3 roles defined — helios-viewer, helios-engineer, helios-admin
- **NetworkPolicies**: Default deny per namespace, explicit allow for inter-component traffic. Collection egress restricted to management CIDRs + specific ports (gNMI 6030/57400, SNMP 161/UDP, flow UDP 2055/6343)
- **Secrets**: External Secrets Operator for ALL credentials. Secrets MUST NOT appear in Git or ConfigMaps.
- **No hardcoded credentials** anywhere
- **Pod Security Standards**: Restricted profile, non-root containers, minimal base images

### Code Quality
- Go: `golangci-lint` clean, proper error handling, context propagation, `slog` structured logging
- Helm: `helm lint` clean, `helm unittest` passing
- Multi-arch: All Dockerfiles must produce amd64+arm64 images
- Coverage: >70% for Go services (SC-009)

### Contract Compliance
- CRD schemas must match `contracts/runbook-crd.yaml` and `contracts/runbookexecution-crd.yaml` exactly
- Protobuf must match `contracts/flow.proto` exactly
- Target Generator output must match `contracts/target-generator-output.yaml` format
- Label taxonomy must match `data-model.md` standard label set

### Edge Cases (from spec.md)
The auditor should verify code handles these edge cases:
- gnmic reconnection with exponential backoff on device disconnect
- goflow2 local buffering when Kafka is unavailable
- Target Generator preserves existing ConfigMaps on NetBox API failure (no empty-write)
- ClickHouse rejects inserts rather than corrupts data when disk is full
- Runbook Operator fails execution clearly when target device is removed from NetBox
- Thanos Query deduplication during Prometheus shard rebalancing
- RunbookExecution stays in PendingApproval indefinitely with alert if no approver available

---

## Working Conventions

### Branching
- `main` — production-ready
- `feature/<phase>-<component>` — feature branches per component
- PRs required, auditor reviews before merge

### Commit Style
- Conventional commits: `feat(gnmic): add K8s clustering config`
- Scope = component name (gnmic, thanos, flow-enricher, runbook-operator, target-generator, etc.)

### Test-First Rule
Within each user story phase, tasks.md defines tests first (marked with ⚠️). These tests MUST:
1. Be written before implementation
2. Be verified to FAIL (confirming they test something real)
3. Pass after implementation is complete

---

## How to Use This Prompt

1. Paste this prompt when starting a Claude Code agent team session
2. The project manager should:
   a. Read `specs/001-helios-platform/tasks.md` to load the full 150-task backlog
   b. Read `specs/001-helios-platform/spec.md` for acceptance criteria
   c. Create team tasks starting with Phase 1 (T001-T010)
   d. Assign tasks to developers respecting [P] parallel markers and dependency order
   e. Route all completed work through the auditor before marking complete
   f. Progress through phases, validating checkpoints before advancing

3. Start with: **"Begin Phase 1: Setup. Project manager, read the spec files and create tasks for the team."**
