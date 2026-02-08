-- Create 1-minute aggregated flows table
CREATE TABLE IF NOT EXISTS flows_1m ON CLUSTER '{cluster}'
(
    timestamp DateTime,
    exporter_site LowCardinality(String),
    exporter_name LowCardinality(String),
    in_if_name LowCardinality(String),
    out_if_name LowCardinality(String),
    src_ip IPv6,
    dst_ip IPv6,
    protocol UInt8,
    src_port UInt16,
    dst_port UInt16,
    src_as UInt32,
    dst_as UInt32,
    src_country LowCardinality(String),
    dst_country LowCardinality(String),
    bytes_sum AggregateFunction(sum, UInt64),
    packets_sum AggregateFunction(sum, UInt64),
    flow_count AggregateFunction(count, UInt64)
) ENGINE = ReplicatedAggregatingMergeTree('/clickhouse/tables/{shard}/flows_1m', '{replica}')
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (exporter_site, exporter_name, timestamp, src_ip, dst_ip)
TTL timestamp + INTERVAL 30 DAY DELETE
SETTINGS index_granularity = 8192;

-- Materialized view for 1-minute aggregation from flows_raw
CREATE MATERIALIZED VIEW IF NOT EXISTS flows_1m_mv ON CLUSTER '{cluster}'
TO flows_1m AS
SELECT
    toStartOfMinute(timestamp) AS timestamp,
    exporter_site,
    exporter_name,
    in_if_name,
    out_if_name,
    src_ip,
    dst_ip,
    protocol,
    src_port,
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
    src_ip,
    dst_ip,
    protocol,
    src_port,
    dst_port,
    src_as,
    dst_as,
    src_country,
    dst_country;
