// AGENTV1 FILE START: standard OTel pdata metrics and bounded JSON batches.
package pipeline

import (
	"errors"
	"github.com/agent-i/agent/internal/platform"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"math"
	"sort"
	"strconv"
	"strings"
)

func Metrics(snapshot platform.Snapshot, resource map[string]string) pmetric.Metrics {
	out := pmetric.NewMetrics()
	rm := out.ResourceMetrics().AppendEmpty()
	for k, v := range resource {
		rm.Resource().Attributes().PutStr(k, v)
	}
	scope := rm.ScopeMetrics().AppendEmpty()
	scope.Scope().SetName("observe-agent.hostmetrics")
	scope.Scope().SetVersion("1")
	metrics := map[string]pmetric.Metric{}
	for _, point := range snapshot.Values {
		if math.IsNaN(point.Value) || math.IsInf(point.Value, 0) || point.Value < 0 || (!strings.HasPrefix(point.Name, "system.") && !strings.HasPrefix(point.Name, "host.")) {
			continue
		}
		key := point.Name + "|" + point.Kind + "|" + point.Unit
		metric, ok := metrics[key]
		if !ok {
			metric = scope.Metrics().AppendEmpty()
			metric.SetName(point.Name)
			metric.SetUnit(point.Unit)
			if point.Kind == "sum" {
				sum := metric.SetEmptySum()
				sum.SetIsMonotonic(true)
				sum.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
			} else {
				metric.SetEmptyGauge()
			}
			metrics[key] = metric
		}
		var dp pmetric.NumberDataPoint
		if point.Kind == "sum" {
			dp = metric.Sum().DataPoints().AppendEmpty()
			if !snapshot.StartTime.IsZero() {
				dp.SetStartTimestamp(pcommon.NewTimestampFromTime(snapshot.StartTime))
				dp.Attributes().PutStr("_boot_time_unix", strconv.FormatInt(snapshot.StartTime.Unix(), 10))
			}
		} else {
			dp = metric.Gauge().DataPoints().AppendEmpty()
		}
		dp.SetTimestamp(pcommon.NewTimestampFromTime(snapshot.ObservedAt))
		dp.SetDoubleValue(point.Value)
		keys := make([]string, 0, len(point.Attributes))
		for k := range point.Attributes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if i >= 16 {
				break
			}
			v := point.Attributes[k]
			if len(k) > 64 || len(v) > 256 {
				continue
			}
			dp.Attributes().PutStr(k, v)
		}
	}
	return out
}

// Batches splits source samples before serialization, below the backend 2000-point limit.
// Oversize individual samples are explicit errors; never silently truncate a request.
func Batches(snapshot platform.Snapshot, resource map[string]string, maxPoints, maxBytes int) ([][]byte, error) {
	if maxPoints < 1 || maxPoints > 1000 || maxBytes < 1024 {
		return nil, errors.New("batch limits invalid")
	}
	result := [][]byte{}
	var encode func([]platform.Measurement) error
	encode = func(points []platform.Measurement) error {
		s := snapshot
		s.Values = points
		raw, e := (&pmetric.JSONMarshaler{}).MarshalMetrics(Metrics(s, resource))
		if e != nil {
			return errors.New("OTLP serialization failed")
		}
		if len(raw) > maxBytes {
			if len(points) <= 1 {
				return errors.New("metric exceeds request bound")
			}
			middle := len(points) / 2
			if e = encode(points[:middle]); e != nil {
				return e
			}
			return encode(points[middle:])
		}
		result = append(result, raw)
		return nil
	}
	for offset := 0; offset < len(snapshot.Values); offset += maxPoints {
		end := offset + maxPoints
		if end > len(snapshot.Values) {
			end = len(snapshot.Values)
		}
		if err := encode(snapshot.Values[offset:end]); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// AGENTV1 FILE END
