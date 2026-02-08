-- Create 1-hour aggregated flows table with /24 network aggregation
CREATE TABLE IF NOT EXISTS flows_1h ON CLUSTER '{cluster}'
(
    timestamp DateTime,
    exporter_site LowCardinality(String),
    exporter_name LowCardinality(String),
    in_if_name LowCardinality(String),
    out_if_name LowCardinality(String),
    src_network IPv6,
    dst_network IPv6,
    protocol UInt8,
    dst_port UInt16,
    src_as UInt32,
    dst_as UInt32,
    src_country LowCardinality(String),
    dst_country LowCardinality(String),
    bytes_sum AggregateFunction(sum, UInt64),
    packets_sum AggregateFunction(sum, UInt64),
    flow_count AggregateFunction(count, UInt64)
) ENGINE = ReplicatedAggregatingMergeTree('/clickhouse/tables/{shard}/flows_1h', '{replica}')
PARTITION BY toYYYYMM(timestamp)
ORDER BY (exporter_site, exporter_name, timestamp, src_network, dst_network)
TTL timestamp + INTERVAL 90 DAY DELETE
SETTINGS index_granularity = 8192;

-- Materialized view for 1-hour aggregation with /24 network grouping
-- IPv4-mapped IPv6 addresses are masked to /24 (last octet zeroed)
-- Native IPv6 addresses are masked to /48
CREATE MATERIALIZED VIEW IF NOT EXISTS flows_1h_mv ON CLUSTER '{cluster}'
TO flows_1h AS
SELECT
    toStartOfHour(timestamp) AS timestamp,
    exporter_site,
    exporter_name,
    in_if_name,
    out_if_name,
    if(ip_version = 4,
       toIPv6(IPv4NumToString(bitAnd(IPv4StringToNum(IPv6NumToString(src_ip)), 0xFFFFFF00))),
       IPv6StringToNum(substring(IPv6NumToString(src_ip), 1, 14) || '::')
    ) AS src_network,
    if(ip_version = 4,
       toIPv6(IPv4NumToString(bitAnd(IPv4StringToNum(IPv6NumToString(dst_ip)), 0xFFFFFF00))),
       IPv6StringToNum(substring(IPv6NumToString(dst_ip), 1, 14) || '::')
    ) AS dst_network,
    protocol,
    dst_port,
    src_as,
    dst_as,
    src_country,
    dst_country,
    sumState(bytes) AS bytes_sum,
    sumState(packets) AS packets_sum,
    countState(bytes) AS flow_count
FROM flows_raw
GROUP BY
    timestamp,
    exporter_site,
    exporter_name,
    in_if_name,
    out_if_name,
    src_network,
    dst_network,
    ip_version,
    protocol,
    dst_port,
    src_as,
    dst_as,
    src_country,
    dst_country;
