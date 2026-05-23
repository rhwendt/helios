#adr #decision #gnmic #collection

# ADR-002: gnmic Deployment Model

**Status**: Accepted
**Date**: 2026-01-09
**Deciders**: rhwendt
**Tags**: #adr #architecture #collection #gnmi

## Context

Helios requires collection of gNMI streaming telemetry from network devices. gnmic is the industry-standard open-source gNMI collector that outputs Prometheus metrics.

Key requirements:
1. **Scale**: Support 50 to 8,000+ network devices
2. **HA**: No single point of failure for collection
3. **Distribution**: Automatically distribute targets across collectors
4. **Efficiency**: Don't duplicate subscriptions (one collector per device)
5. **Dynamic**: Handle device additions/removals without restart

gnmic supports clustering for high availability and target distribution, but we need to decide how to deploy it in Kubernetes.

## Decision

We will deploy gnmic as a **StatefulSet with Kubernetes-based clustering**.

### Architecture

```
+------------------+     +------------------+     +------------------+
|   gnmic-0        |     |   gnmic-1        |     |   gnmic-N        |
|   (StatefulSet)  |     |   (StatefulSet)  |     |   (StatefulSet)  |
+--------+---------+     +--------+---------+     +--------+---------+
         |                        |                        |
         +------------------------+------------------------+
                                  |
                    +-------------v-------------+
                    |    K8s Lease (Locker)     |
                    |    Leader Election        |
                    |    Target Distribution    |
                    +---------------------------+
                                  |
                    +-------------v-------------+
                    |   ConfigMap (Targets)     |
                    |   From NetBox Sync        |
                    +---------------------------+
```

### Key Configuration

```yaml
# gnmic clustering configuration
clustering:
  cluster-name: helios-gnmic
  instance-name: ${POD_NAME}
  service-address: ${POD_IP}:57400
  locker:
    type: k8s  # Uses K8s leases for leader election
```

### Scaling

| Scale | gnmic Replicas | Targets/Pod |
|-------|----------------|-------------|
| Small (<50) | 2 | ~25 |
| Medium (50-500) | 3-5 | ~100-150 |
| Large (500-2000) | 5-10 | ~200-400 |
| XL (2000-8000) | 10-20 | ~400-500 |

## Consequences

### Positive

1. **Automatic Target Distribution**: gnmic clustering handles balancing targets across replicas
2. **No Duplicate Subscriptions**: Each device is assigned to exactly one gnmic instance
3. **Self-Healing**: If a pod fails, targets are redistributed to remaining pods
4. **Native K8s Integration**: Uses K8s leases, no external coordination service needed
5. **HPA Compatible**: Can scale based on subscription count or CPU usage
6. **StatefulSet Benefits**: Stable network identity, ordered deployment/scaling
7. **Simple Configuration**: Single ConfigMap for all targets, clustering handles distribution

### Negative

1. **StatefulSet Limitations**: Slower scaling than Deployment (ordered)
2. **Clustering Overhead**: gRPC communication between instances for coordination
3. **Target Rebalancing Delay**: Brief gap during failover/scale events
4. **Single Target ConfigMap**: Very large target lists may hit ConfigMap size limits

### Neutral

1. **Prometheus Metrics Port**: Each pod exposes :9804 for scraping
2. **Memory per Target**: ~1-2 MB per active subscription

## Alternatives Considered

### Option 1: DaemonSet Deployment

**Description**: Deploy gnmic on every K8s node, use node-local targets

**Pros**:
- Simple deployment model
- Predictable pod placement
- No clustering needed

**Cons**:
- Wastes resources on nodes not collecting
- No target balancing (some nodes may have more targets)
- Coupling to node topology

**Why Not Selected**: Inefficient resource usage and no automatic target distribution.

### Option 2: Deployment (Non-Clustered)

**Description**: Deploy gnmic as a Deployment with each pod collecting all targets

**Pros**:
- Simple scaling
- No clustering complexity

**Cons**:
- All pods subscribe to all devices (N copies of every subscription)
- Wastes device resources (N connections per device)
- Wastes cluster resources (duplicate work)

**Why Not Selected**: Duplicate subscriptions violate the efficiency requirement.

### Option 3: Deployment with External Coordinator

**Description**: Use Deployment + external service (Consul, etcd) for target distribution

**Pros**:
- Faster scaling than StatefulSet
- More flexible placement

**Cons**:
- Requires external coordinator dependency
- Additional operational complexity
- gnmic native K8s locker is simpler

**Why Not Selected**: K8s-native locker eliminates need for external dependency.

### Option 4: VM-Based Deployment (Outside K8s)

**Description**: Deploy gnmic on dedicated VMs

**Pros**:
- Full control over placement
- Can be closer to network devices
- Simpler for network team

**Cons**:
- Not cloud-native
- Manual scaling and management
- Separate deployment pipeline
- Harder to integrate with K8s ecosystem

**Why Not Selected**: Violates cloud-native design principle. Consider only if K8s cannot reach all network devices.

## Implementation Notes

### HorizontalPodAutoscaler

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: gnmic-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: StatefulSet
    name: gnmic
  minReplicas: 2
  maxReplicas: 20
  metrics:
  - type: Pods
    pods:
      metric:
        name: gnmic_subscription_active_count
      target:
        type: AverageValue
        averageValue: "500"
```

### Resource Sizing

```yaml
# Small scale
resources:
  requests:
    cpu: 250m
    memory: 256Mi
  limits:
    cpu: 1000m
    memory: 1Gi

# XL scale
resources:
  requests:
    cpu: 1000m
    memory: 2Gi
  limits:
    cpu: 4000m
    memory: 8Gi
```

### Health Probes

```yaml
livenessProbe:
  httpGet:
    path: /api/healthz
    port: 7890
  initialDelaySeconds: 10
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /api/targets
    port: 7890
  initialDelaySeconds: 5
  periodSeconds: 5
```

## References

- [gnmic Clustering Documentation](https://gnmic.openconfig.net/user_guide/HA/)
- [gnmic Kubernetes Locker](https://gnmic.openconfig.net/user_guide/HA/locker/k8s/)
- [gnmic Prometheus Output](https://gnmic.openconfig.net/user_guide/outputs/prometheus_output/)
- [[../architecture/architecture]] - Full architecture document
- [[adr-001-storage-backend]] - Storage backend decision
