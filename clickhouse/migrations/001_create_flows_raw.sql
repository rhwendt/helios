-- Create Kafka engine table for consuming enriched flows
CREATE TABLE IF NOT EXISTS flows_kafka ON CLUSTER '{cluster}'
(
    timestamp DateTime64(3),
    flow_type Enum8('unknown' = 0, 'netflow_v5' = 1, 'netflow_v9' = 2, 'ipfix' = 3, 'sflow' = 4),
    exporter_ip IPv4,
    exporter_name LowCardinality(String),
    exporter_site LowCardinality(String),
    exporter_region LowCardinality(String),
    exporter_role LowCardinality(String),
    in_if UInt32,
    out_if UInt32,
    in_if_name LowCardinality(String),
    out_if_name LowCardinality(String),
    in_if_speed UInt64,
    out_if_speed UInt64,
    src_ip IPv6,
    dst_ip IPv6,
    ip_version UInt8,
    protocol UInt8,
    src_port UInt16,
    dst_port UInt16,
    tcp_flags UInt8,
    bytes UInt64,
    packets UInt64,
    sampling_rate UInt32,
    src_as UInt32,
    dst_as UInt32,
    next_hop IPv4,
    src_country LowCardinality(String),
    dst_country LowCardinality(String),
    src_city LowCardinality(String),
    dst_city LowCardinality(String),
    src_as_name LowCardinality(String),
    dst_as_name LowCardinality(String),
    src_vlan UInt16,
    dst_vlan UInt16,
    direction Enum8('unknown' = 0, 'ingress' = 1, 'egress' = 2)
) ENGINE = Kafka()
SETTINGS
    kafka_broker_list = 'helios-flows-kafka-bootstrap.helios-flows.svc.cluster.local:9092',
    kafka_topic_list = 'helios-flows-enriched',
    kafka_group_name = 'clickhouse-flows-consumer',
    kafka_format = 'Protobuf',
    kafka_schema = 'flow.proto:helios.flows.EnrichedFlow',
    kafka_num_consumers = 2;

-- Create the raw flows table with ReplicatedMergeTree
CREATE TABLE IF NOT EXISTS flows_raw ON CLUSTER '{cluster}'
(
    timestamp DateTime64(3),
    flow_type Enum8('unknown' = 0, 'netflow_v5' = 1, 'netflow_v9' = 2, 'ipfix' = 3, 'sflow' = 4),
    exporter_ip IPv4,
    exporter_name LowCardinality(String),
    exporter_site LowCardinality(String),
    exporter_region LowCardinality(String),
    exporter_role LowCardinality(String),
    in_if UInt32,
    out_if UInt32,
    in_if_name LowCardinality(String),
    out_if_name LowCardinality(String),
    in_if_speed UInt64,
    out_if_speed UInt64,
    src_ip IPv6,
    dst_ip IPv6,
    ip_version UInt8,
    protocol UInt8,
    src_port UInt16,
    dst_port UInt16,
    tcp_flags UInt8,
    bytes UInt64,
    packets UInt64,
    sampling_rate UInt32,
    src_as UInt32,
    dst_as UInt32,
    next_hop IPv4,
    src_country LowCardinality(String),
    dst_country LowCardinality(String),
    src_city LowCardinality(String),
    dst_city LowCardinality(String),
    src_as_name LowCardinality(String),
    dst_as_name LowCardinality(String),
    src_vlan UInt16,
    dst_vlan UInt16,
    direction Enum8('unknown' = 0, 'ingress' = 1, 'egress' = 2)
) ENGINE = ReplicatedMergeTree('/clickhouse/tables/{shard}/flows_raw', '{replica}')
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (exporter_site, exporter_name, timestamp, src_ip, dst_ip)
TTL timestamp + INTERVAL 7 DAY DELETE
SETTINGS index_granularity = 8192;

-- Materialized view to pipe Kafka data into flows_raw
CREATE MATERIALIZED VIEW IF NOT EXISTS flows_raw_mv ON CLUSTER '{cluster}'
TO flows_raw AS
SELECT * FROM flows_kafka;
