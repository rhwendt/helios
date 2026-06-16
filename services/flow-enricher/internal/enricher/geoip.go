package enricher

import (
	"fmt"
	"log/slog"
	"net"

	"github.com/oschwald/maxminddb-golang"
)

// GeoIPResult holds the result of a GeoIP lookup.
type GeoIPResult struct {
	Country string
	City    string
	ASNum   uint32
	ASName  string
}

// GeoIPReader provides IP-to-location and IP-to-ASN lookups.
type GeoIPReader struct {
	cityDB *maxminddb.Reader
	asnDB  *maxminddb.Reader
	logger *slog.Logger
}

// cityRecord mirrors the MaxMind GeoLite2-City database record structure.
type cityRecord struct {
	Country struct {
		ISOCode string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
	City struct {
		Names map[string]string `maxminddb:"names"`
	} `maxminddb:"city"`
}

// asnRecord mirrors the MaxMind GeoLite2-ASN database record structure.
type asnRecord struct {
	AutonomousSystemNumber       uint32 `maxminddb:"autonomous_system_number"`
	AutonomousSystemOrganization string `maxminddb:"autonomous_system_organization"`
}

// NewGeoIPReader opens the MaxMind GeoLite2 databases.
func NewGeoIPReader(cityDBPath, asnDBPath string, logger *slog.Logger) (*GeoIPReader, error) {
	cityDB, err := maxminddb.Open(cityDBPath)
	if err != nil {
		return nil, fmt.Errorf("opening city database: %w", err)
	}

	asnDB, err := maxminddb.Open(asnDBPath)
	if err != nil {
		_ = cityDB.Close()
		return nil, fmt.Errorf("opening ASN database: %w", err)
	}

	return &GeoIPReader{
		cityDB: cityDB,
		asnDB:  asnDB,
		logger: logger,
	}, nil
}

// Lookup performs a GeoIP lookup for the given IP address.
func (r *GeoIPReader) Lookup(ip net.IP) GeoIPResult {
	var result GeoIPResult

	var city cityRecord
	if err := r.cityDB.Lookup(ip, &city); err != nil {
		r.logger.Debug("city lookup failed", "ip", ip, "error", err)
	} else {
		result.Country = city.Country.ISOCode
		if name, ok := city.City.Names["en"]; ok {
			result.City = name
		}
	}

	var asn asnRecord
	if err := r.asnDB.Lookup(ip, &asn); err != nil {
		r.logger.Debug("ASN lookup failed", "ip", ip, "error", err)
	} else {
		result.ASNum = asn.AutonomousSystemNumber
		result.ASName = asn.AutonomousSystemOrganization
	}

	return result
}

// Close releases the database resources.
func (r *GeoIPReader) Close() error {
	var errs []error
	if err := r.cityDB.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := r.asnDB.Close(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("closing GeoIP databases: %v", errs)
	}
	return nil
}
