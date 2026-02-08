package enricher

import (
	"log/slog"
	"net"

	flowpb "github.com/rhwendt/helios/services/flow-enricher/internal/proto"
)

// Enricher applies NetBox metadata and GeoIP data to raw flow records.
type Enricher struct {
	netbox *NetBoxCache
	geoip  *GeoIPReader
	logger *slog.Logger
}

// New creates a new Enricher with the given dependencies.
func New(netbox *NetBoxCache, geoip *GeoIPReader, logger *slog.Logger) *Enricher {
	return &Enricher{
		netbox: netbox,
		geoip:  geoip,
		logger: logger,
	}
}

// Enrich takes a raw flow protobuf and applies NetBox metadata and GeoIP enrichment.
func (e *Enricher) Enrich(flow *flowpb.EnrichedFlow) *flowpb.EnrichedFlow {
	e.applyNetBoxMetadata(flow)
	e.applyGeoIP(flow)
	return flow
}

// applyNetBoxMetadata enriches the flow with device and interface metadata from NetBox.
func (e *Enricher) applyNetBoxMetadata(flow *flowpb.EnrichedFlow) {
	exporterIP := uint32ToIP(flow.ExporterIp)
	device, ok := e.netbox.LookupByIP(exporterIP)
	if !ok {
		e.logger.Debug("no NetBox metadata for exporter", "ip", exporterIP)
		return
	}

	flow.ExporterName = device.Name
	flow.ExporterSite = device.Site
	flow.ExporterRegion = device.Region
	flow.ExporterRole = device.Role

	if iface, ok := device.Interfaces[flow.InIf]; ok {
		flow.InIfName = iface.Name
		flow.InIfSpeed = iface.Speed
	}
	if iface, ok := device.Interfaces[flow.OutIf]; ok {
		flow.OutIfName = iface.Name
		flow.OutIfSpeed = iface.Speed
	}
}

// applyGeoIP enriches the flow with GeoIP country/city/ASN data.
func (e *Enricher) applyGeoIP(flow *flowpb.EnrichedFlow) {
	if e.geoip == nil {
		return
	}

	if len(flow.SrcIp) > 0 {
		srcIP := net.IP(flow.SrcIp)
		srcResult := e.geoip.Lookup(srcIP)
		flow.SrcCountry = srcResult.Country
		flow.SrcCity = srcResult.City
		flow.SrcAsName = srcResult.ASName
		if flow.SrcAs == 0 {
			flow.SrcAs = srcResult.ASNum
		}
	}

	if len(flow.DstIp) > 0 {
		dstIP := net.IP(flow.DstIp)
		dstResult := e.geoip.Lookup(dstIP)
		flow.DstCountry = dstResult.Country
		flow.DstCity = dstResult.City
		flow.DstAsName = dstResult.ASName
		if flow.DstAs == 0 {
			flow.DstAs = dstResult.ASNum
		}
	}
}

// uint32ToIP converts a fixed32 IP to net.IP.
func uint32ToIP(ip uint32) net.IP {
	return net.IPv4(
		byte(ip>>24),
		byte(ip>>16),
		byte(ip>>8),
		byte(ip),
	)
}
