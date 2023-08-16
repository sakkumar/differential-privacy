//
// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

// This file contains methods & ParDos used by multiple DP aggregations.

package pbeam

import (
	"bytes"
	"fmt"
	"math"
	"math/rand"
	"reflect"

	"github.com/google/differential-privacy/go/v2/checks"
	"github.com/google/differential-privacy/go/v2/dpagg"
	"github.com/google/differential-privacy/go/v2/noise"
	"github.com/google/differential-privacy/privacy-on-beam/v2/internal/kv"
	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/core/typex"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/register"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/transforms/top"
)

func init() {
	register.Combiner3[boundedSumAccumInt64, int64, *int64](&boundedSumInt64Fn{})
	register.Combiner3[boundedSumAccumFloat64, float64, *float64](&boundedSumFloat64Fn{})
	register.Combiner3[expandValuesAccum, beam.V, [][]byte](&expandValuesCombineFn{})
	register.Combiner3[expandFloat64ValuesAccum, float64, []float64](&expandFloat64ValuesCombineFn{})

	register.DoFn1x3[pairInt64, beam.W, int64, error](&decodePairInt64Fn{})
	register.DoFn1x3[pairFloat64, beam.W, float64, error](&decodePairFloat64Fn{})
	register.DoFn2x3[beam.U, kv.Pair, beam.U, beam.W, error](&dropValuesFn{})
	register.DoFn2x3[kv.Pair, []byte, beam.W, kv.Pair, error](&encodeKVFn{})
	register.DoFn2x3[beam.W, kv.Pair, kv.Pair, beam.V, error](&encodeIDKFn{})
	register.DoFn2x3[kv.Pair, beam.V, beam.W, kv.Pair, error](&decodeIDKFn{})
	register.DoFn1x3[pairArrayFloat64, beam.W, []float64, error](&decodePairArrayFloat64Fn{})

	register.Function2x1[beam.V, beam.V, bool](randBool)
	register.Function3x0[beam.W, []beam.V, func(beam.W, beam.V)](flattenValues)
	register.Emitter2[beam.W, beam.V]()
	register.Function2x2[kv.Pair, int64, []byte, pairInt64](rekeyInt64)
	register.Function2x2[kv.Pair, float64, []byte, pairFloat64](rekeyFloat64)
	register.Function2x2[kv.Pair, []float64, []byte, pairArrayFloat64](rekeyArrayFloat64)
	register.Function2x2[beam.V, int64, beam.V, int64](clampNegativePartitionsInt64)
	register.Function2x2[beam.V, float64, beam.V, float64](clampNegativePartitionsFloat64)
	register.Function3x0[beam.V, *int64, func(beam.V, int64)](dropThresholdedPartitionsInt64)
	register.Emitter2[beam.V, int64]()
	register.Function3x0[beam.V, *float64, func(beam.V, float64)](dropThresholdedPartitionsFloat64)
	register.Emitter2[beam.V, float64]()
	register.Function3x0[beam.V, []float64, func(beam.V, []float64)](dropThresholdedPartitionsFloat64Slice)
	register.Emitter2[beam.V, []float64]()
	register.Function2x2[beam.W, *int64, beam.W, int64](dereferenceValueToInt64)
	register.Function2x2[beam.W, *float64, beam.W, float64](dereferenceValueToFloat64)
	register.Function2x2[kv.Pair, int, kv.Pair, float64](convertIntToFloat64)
	register.Function2x2[kv.Pair, int8, kv.Pair, float64](convertInt8ToFloat64)
	register.Function2x2[kv.Pair, int16, kv.Pair, float64](convertInt16ToFloat64)
	register.Function2x2[kv.Pair, int32, kv.Pair, float64](convertInt32ToFloat64)
	register.Function2x2[kv.Pair, int64, kv.Pair, float64](convertInt64ToFloat64)
	register.Function2x2[kv.Pair, uint, kv.Pair, float64](convertUintToFloat64)
	register.Function2x2[kv.Pair, uint8, kv.Pair, float64](convertUint8ToFloat64)
	register.Function2x2[kv.Pair, uint16, kv.Pair, float64](convertUint16ToFloat64)
	register.Function2x2[kv.Pair, uint32, kv.Pair, float64](convertUint32ToFloat64)
	register.Function2x2[kv.Pair, uint64, kv.Pair, float64](convertUint64ToFloat64)
	register.Function2x2[kv.Pair, float32, kv.Pair, float64](convertFloat32ToFloat64)
	register.Function2x2[kv.Pair, float64, kv.Pair, float64](convertFloat64ToFloat64)
}

// randBool returns a uniformly random boolean. The randomness used here is not
// cryptographically secure, and using this with top.LargestPerKey doesn't
// necessarily result in a uniformly random permutation: the distribution of
// the permutation depends on the exact sorting algorithm used by Beam and the
// order in which the input values are processed within the pipeline.
//
// The fact that the resulting permutation is not necessarily uniformly random is
// not a problem, since all we require from this function to satisfy DP properties
// is that it doesn't depend on the data. More specifically, in order to satisfy DP
// properties, a privacy unit's data should not influence another privacy unit's
// permutation of contributions. We assume that the order Beam processes the
// input values for a privacy unit is independent of other privacy units'
// inputs, in which case this requirement is satisfied.
func randBool(_, _ beam.V) bool {
	return rand.Uint32()%2 == 0
}

// boundContributions takes a PCollection<K,V> as input, and for each key, selects and returns
// at most contributionLimit records with this key. The selection is "mostly random":
// the records returned are selected randomly, but the randomness isn't secure.
// This is fine to use in the cross-partition bounding stage or in the per-partition bounding stage,
// since the privacy guarantee doesn't depend on the privacy unit contributions being selected randomly.
//
// In order to do the cross-partition contribution bounding we need:
//  1. the key to be the privacy ID.
//  2. the value to be the partition ID or the pair = {partition ID, aggregated statistic},
//     where aggregated statistic is either array of values which are associated with the given id
//     and partition, or sum/count/etc of these values.
//
// In order to do the per-partition contribution bounding we need:
//  1. the key to be the pair = {privacy ID, partition ID}.
//  2. the value to be just the value which is associated with that {privacy ID, partition ID} pair
//     (there could be multiple entries with the same key).
func boundContributions(s beam.Scope, kvCol beam.PCollection, contributionLimit int64) beam.PCollection {
	s = s.Scope("boundContributions")
	// Transform the PCollection<K,V> into a PCollection<K,[]V>, where
	// there are at most contributionLimit elements per slice, chosen randomly. To
	// do that, the easiest solution seems to be to use the LargestPerKey
	// function (that returns the contributionLimit "largest" elements), except
	// the function used to sort elements is random.
	sampled := top.LargestPerKey(s, kvCol, int(contributionLimit), randBool)
	// Flatten the values for each key to get back a PCollection<K,V>.
	return beam.ParDo(s, flattenValues, sampled)
}

// Given a PCollection<K,[]V>, flattens the second argument to return a PCollection<K,V>.
func flattenValues(key beam.W, values []beam.V, emit func(beam.W, beam.V)) {
	for _, v := range values {
		emit(key, v)
	}
}

func findRekeyFn(kind reflect.Kind) (any, error) {
	switch kind {
	case reflect.Int64:
		return rekeyInt64, nil
	case reflect.Float64:
		return rekeyFloat64, nil
	default:
		return nil, fmt.Errorf("kind(%v) should be int64 or float64", kind)
	}
}

// pairInt64 contains an encoded partition key and an int64 metric.
type pairInt64 struct {
	K []byte
	M int64
}

// rekeyInt64 transforms a PCollection<kv.Pair<codedK,codedV>,int64> into a
// PCollection<codedK,pairInt64<codedV,int>>.
func rekeyInt64(kv kv.Pair, m int64) ([]byte, pairInt64) {
	return kv.K, pairInt64{kv.V, m}
}

// pairFloat64 contains an encoded value and an float64 metric.
type pairFloat64 struct {
	K []byte
	M float64
}

// rekeyFloat64 transforms a PCollection<kv.Pair<codedK,codedV>,float64> into a
// PCollection<codedK,pairFloat64<codedV,int>>.
func rekeyFloat64(kv kv.Pair, m float64) ([]byte, pairFloat64) {
	return kv.K, pairFloat64{kv.V, m}
}

// pairArrayFloat64 contains an encoded partition key and a slice of float64 metrics.
type pairArrayFloat64 struct {
	K []byte
	M []float64
}

// rekeyArrayFloat64 transforms a PCollection<kv.Pair<codedK,codedV>,[]float64> into a
// PCollection<codedK,pairArrayFloat64<codedV,[]float64>>.
func rekeyArrayFloat64(kv kv.Pair, m []float64) ([]byte, pairArrayFloat64) {
	return kv.K, pairArrayFloat64{kv.V, m}
}

func newDecodePairFn(t reflect.Type, kind reflect.Kind) (any, error) {
	switch kind {
	case reflect.Int64:
		return newDecodePairInt64Fn(t), nil
	case reflect.Float64:
		return newDecodePairFloat64Fn(t), nil
	default:
		return nil, fmt.Errorf("kind(%v) should be int64 or float64", kind)
	}
}

// decodePairInt64Fn transforms a PCollection<pairInt64<KX,int64>> into a
// PCollection<K,int64>.
type decodePairInt64Fn struct {
	KType beam.EncodedType
	kDec  beam.ElementDecoder
}

func newDecodePairInt64Fn(t reflect.Type) *decodePairInt64Fn {
	return &decodePairInt64Fn{KType: beam.EncodedType{t}}
}

func (fn *decodePairInt64Fn) Setup() {
	fn.kDec = beam.NewElementDecoder(fn.KType.T)
}

func (fn *decodePairInt64Fn) ProcessElement(pair pairInt64) (beam.W, int64, error) {
	k, err := fn.kDec.Decode(bytes.NewBuffer(pair.K))
	if err != nil {
		return nil, 0, fmt.Errorf("pbeam.decodePairInt64Fn.ProcessElement: couldn't decode pair %v: %w", pair, err)
	}
	return k, pair.M, nil
}

// decodePairFloat64Fn transforms a PCollection<pairFloat64<codedK,float64>> into a
// PCollection<K,float64>.
type decodePairFloat64Fn struct {
	KType beam.EncodedType
	kDec  beam.ElementDecoder
}

func newDecodePairFloat64Fn(t reflect.Type) *decodePairFloat64Fn {
	return &decodePairFloat64Fn{KType: beam.EncodedType{t}}
}

func (fn *decodePairFloat64Fn) Setup() {
	fn.kDec = beam.NewElementDecoder(fn.KType.T)
}

func (fn *decodePairFloat64Fn) ProcessElement(pair pairFloat64) (beam.W, float64, error) {
	k, err := fn.kDec.Decode(bytes.NewBuffer(pair.K))
	if err != nil {
		return nil, 0.0, fmt.Errorf("pbeam.decodePairFloat64Fn.ProcessElement: couldn't decode pair %v: %w", pair, err)
	}
	return k, pair.M, nil
}

// decodePairArrayFloat64Fn transforms a PCollection<pairArrayFloat64<codedK,[]float64>> into a
// PCollection<K,[]float64>.
type decodePairArrayFloat64Fn struct {
	KType beam.EncodedType
	kDec  beam.ElementDecoder
}

func newDecodePairArrayFloat64Fn(t reflect.Type) *decodePairArrayFloat64Fn {
	return &decodePairArrayFloat64Fn{KType: beam.EncodedType{t}}
}

func (fn *decodePairArrayFloat64Fn) Setup() {
	fn.kDec = beam.NewElementDecoder(fn.KType.T)
}

func (fn *decodePairArrayFloat64Fn) ProcessElement(pair pairArrayFloat64) (beam.W, []float64, error) {
	k, err := fn.kDec.Decode(bytes.NewBuffer(pair.K))
	if err != nil {
		return nil, nil, fmt.Errorf("pbeam.decodePairArrayFloat64Fn.ProcessElement: couldn't decode pair %v: %w", pair, err)
	}
	return k, pair.M, nil
}

// newBoundedSumFn returns a boundedSumInt64Fn or boundedSumFloat64Fn depending on vKind.
func newBoundedSumFn(epsilon, delta float64, maxPartitionsContributed int64, lower, upper float64, noiseKind noise.Kind, vKind reflect.Kind, publicPartitions bool, testMode TestMode) (any, error) {
	var err, checkErr error
	var bsFn any

	switch vKind {
	case reflect.Int64:
		checkErr = checks.CheckBoundsFloat64AsInt64(lower, upper)
		if checkErr != nil {
			return nil, checkErr
		}
		bsFn, err = newBoundedSumInt64Fn(epsilon, delta, maxPartitionsContributed, int64(lower), int64(upper), noiseKind, publicPartitions, testMode)
	case reflect.Float64:
		checkErr = checks.CheckBoundsFloat64(lower, upper)
		if checkErr != nil {
			return nil, checkErr
		}
		bsFn, err = newBoundedSumFloat64Fn(epsilon, delta, maxPartitionsContributed, lower, upper, noiseKind, publicPartitions, testMode)
	default:
		err = fmt.Errorf("vKind(%v) should be int64 or float64", vKind)
	}

	return bsFn, err
}

// newBoundedSumFn returns a boundedSumInt64Fn or boundedSumFloat64Fn depending on vKind.
//
// Uses the new privacy budget API where clients specify aggregation budget and partition selection budget separately.
func newBoundedSumFnTemp(spec PrivacySpec, params SumParams, noiseKind noise.Kind, vKind reflect.Kind, publicPartitions bool) (any, error) {
	var err, checkErr error
	var bsFn any
	switch vKind {
	case reflect.Int64:
		checkErr = checks.CheckBoundsFloat64AsInt64(params.MinValue, params.MaxValue)
		if checkErr != nil {
			return nil, checkErr
		}
		bsFn, err = newBoundedSumInt64FnTemp(params.AggregationEpsilon, params.AggregationDelta, params.PartitionSelectionParams.Epsilon, params.PartitionSelectionParams.Delta, spec.preThreshold, params.MaxPartitionsContributed, int64(params.MinValue), int64(params.MaxValue), noiseKind, publicPartitions, spec.testMode)
	case reflect.Float64:
		checkErr = checks.CheckBoundsFloat64(params.MinValue, params.MaxValue)
		if checkErr != nil {
			return nil, checkErr
		}
		bsFn, err = newBoundedSumFloat64FnTemp(params.AggregationEpsilon, params.AggregationDelta, params.PartitionSelectionParams.Epsilon, params.PartitionSelectionParams.Delta, spec.preThreshold, params.MaxPartitionsContributed, params.MinValue, params.MaxValue, noiseKind, publicPartitions, spec.testMode)
	default:
		err = fmt.Errorf("vKind(%v) should be int64 or float64", vKind)
	}

	return bsFn, err
}

type boundedSumAccumInt64 struct {
	BS               *dpagg.BoundedSumInt64
	SP               *dpagg.PreAggSelectPartition
	PublicPartitions bool
}

// boundedSumInt64Fn is a differentially private combineFn for summing values. Do not
// initialize it yourself, use newBoundedSumInt64Fn to create a boundedSumInt64Fn instance.
type boundedSumInt64Fn struct {
	// Privacy spec parameters (set during initial construction).
	NoiseEpsilon              float64
	PartitionSelectionEpsilon float64
	NoiseDelta                float64
	PartitionSelectionDelta   float64
	PreThreshold              int64
	MaxPartitionsContributed  int64
	Lower                     int64
	Upper                     int64
	NoiseKind                 noise.Kind
	noise                     noise.Noise // Set during Setup phase according to NoiseKind.
	PublicPartitions          bool
	TestMode                  TestMode
}

// newBoundedSumInt64Fn returns a boundedSumInt64Fn with the given budget and parameters.
func newBoundedSumInt64Fn(epsilon, delta float64, maxPartitionsContributed, lower, upper int64, noiseKind noise.Kind, publicPartitions bool, testMode TestMode) (*boundedSumInt64Fn, error) {
	fn := &boundedSumInt64Fn{
		MaxPartitionsContributed: maxPartitionsContributed,
		Lower:                    lower,
		Upper:                    upper,
		NoiseKind:                noiseKind,
		PublicPartitions:         publicPartitions,
		TestMode:                 testMode,
	}
	if fn.PublicPartitions {
		fn.NoiseEpsilon = epsilon
		fn.NoiseDelta = delta
		return fn, nil
	}
	fn.NoiseEpsilon = epsilon / 2
	fn.PartitionSelectionEpsilon = epsilon - fn.NoiseEpsilon
	switch noiseKind {
	case noise.GaussianNoise:
		fn.NoiseDelta = delta / 2
	case noise.LaplaceNoise:
		fn.NoiseDelta = 0
	default:
		return nil, fmt.Errorf("unknown noise.Kind (%v) is specified. Please specify a valid noise", noiseKind)
	}
	fn.PartitionSelectionDelta = delta - fn.NoiseDelta
	return fn, nil
}

// newBoundedSumInt64Fn returns a boundedSumInt64Fn with the given budget and parameters.
//
// Uses the new privacy budget API where clients specify aggregation budget and partition selection budget separately.
func newBoundedSumInt64FnTemp(aggregationEpsilon, aggregationDelta, partitionSelectionEpsilon, partitionSelectionDelta float64, preThreshold, maxPartitionsContributed, lower, upper int64, noiseKind noise.Kind, publicPartitions bool, testMode TestMode) (*boundedSumInt64Fn, error) {
	if noiseKind != noise.GaussianNoise && noiseKind != noise.LaplaceNoise {
		return nil, fmt.Errorf("unknown noise.Kind (%v) is specified. Please specify a valid noise", noiseKind)
	}
	return &boundedSumInt64Fn{
		NoiseEpsilon:              aggregationEpsilon,
		NoiseDelta:                aggregationDelta,
		PartitionSelectionEpsilon: partitionSelectionEpsilon,
		PartitionSelectionDelta:   partitionSelectionDelta,
		PreThreshold:              preThreshold,
		MaxPartitionsContributed:  maxPartitionsContributed,
		Lower:                     lower,
		Upper:                     upper,
		NoiseKind:                 noiseKind,
		PublicPartitions:          publicPartitions,
		TestMode:                  testMode,
	}, nil
}

func (fn *boundedSumInt64Fn) Setup() {
	fn.noise = noise.ToNoise(fn.NoiseKind)
	if fn.TestMode.isEnabled() {
		fn.noise = noNoise{}
	}
}

func (fn *boundedSumInt64Fn) CreateAccumulator() (boundedSumAccumInt64, error) {
	if fn.TestMode == NoNoiseWithoutContributionBounding {
		fn.Lower = math.MinInt64
		fn.Upper = math.MaxInt64
	}
	var bs *dpagg.BoundedSumInt64
	var err error
	bs, err = dpagg.NewBoundedSumInt64(&dpagg.BoundedSumInt64Options{
		Epsilon:                  fn.NoiseEpsilon,
		Delta:                    fn.NoiseDelta,
		MaxPartitionsContributed: fn.MaxPartitionsContributed,
		Lower:                    fn.Lower,
		Upper:                    fn.Upper,
		Noise:                    fn.noise,
	})
	if err != nil {
		return boundedSumAccumInt64{}, err
	}
	accum := boundedSumAccumInt64{BS: bs, PublicPartitions: fn.PublicPartitions}
	if !fn.PublicPartitions {
		accum.SP, err = dpagg.NewPreAggSelectPartition(&dpagg.PreAggSelectPartitionOptions{
			Epsilon:                  fn.PartitionSelectionEpsilon,
			Delta:                    fn.PartitionSelectionDelta,
			PreThreshold:             fn.PreThreshold,
			MaxPartitionsContributed: fn.MaxPartitionsContributed,
		})
	}
	return accum, err
}

func (fn *boundedSumInt64Fn) AddInput(a boundedSumAccumInt64, value int64) (boundedSumAccumInt64, error) {
	err := a.BS.Add(value)
	if err != nil {
		return a, err
	}
	if !fn.PublicPartitions {
		err := a.SP.Increment()
		if err != nil {
			return a, err
		}
	}
	return a, nil
}

func (fn *boundedSumInt64Fn) MergeAccumulators(a, b boundedSumAccumInt64) (boundedSumAccumInt64, error) {
	err := a.BS.Merge(b.BS)
	if err != nil {
		return a, err
	}
	if !fn.PublicPartitions {
		err := a.SP.Merge(b.SP)
		if err != nil {
			return a, err
		}
	}
	return a, nil
}

func (fn *boundedSumInt64Fn) ExtractOutput(a boundedSumAccumInt64) (*int64, error) {
	if fn.TestMode.isEnabled() {
		a.BS.Noise = noNoise{}
	}
	var err error
	shouldKeepPartition := fn.TestMode.isEnabled() || a.PublicPartitions // If in test mode or public partitions are specified, we always keep the partition.
	if !shouldKeepPartition {                                            // If not, we need to perform private partition selection.
		shouldKeepPartition, err = a.SP.ShouldKeepPartition()
		if err != nil {
			return nil, err
		}
	}

	if shouldKeepPartition {
		result, err := a.BS.Result()
		return &result, err
	}
	return nil, nil
}

func (fn *boundedSumInt64Fn) String() string {
	return fmt.Sprintf("%#v", fn)
}

type boundedSumAccumFloat64 struct {
	BS               *dpagg.BoundedSumFloat64
	SP               *dpagg.PreAggSelectPartition
	PublicPartitions bool
}

// boundedSumFloat64Fn is a differentially private combineFn for summing values. Do not
// initialize it yourself, use newBoundedSumFloat64Fn to create a boundedSumFloat64Fn instance.
type boundedSumFloat64Fn struct {
	// Privacy spec parameters (set during initial construction).
	NoiseEpsilon              float64
	PartitionSelectionEpsilon float64
	NoiseDelta                float64
	PartitionSelectionDelta   float64
	PreThreshold              int64
	MaxPartitionsContributed  int64
	Lower                     float64
	Upper                     float64
	NoiseKind                 noise.Kind
	// Noise, set during Setup phase according to NoiseKind.
	noise            noise.Noise
	PublicPartitions bool
	TestMode         TestMode
}

// newBoundedSumFloat64Fn returns a boundedSumFloat64Fn with the given budget and parameters.
func newBoundedSumFloat64Fn(epsilon, delta float64, maxPartitionsContributed int64, lower, upper float64, noiseKind noise.Kind, publicPartitions bool, testMode TestMode) (*boundedSumFloat64Fn, error) {
	fn := &boundedSumFloat64Fn{
		MaxPartitionsContributed: maxPartitionsContributed,
		Lower:                    lower,
		Upper:                    upper,
		NoiseKind:                noiseKind,
		PublicPartitions:         publicPartitions,
		TestMode:                 testMode,
	}
	if fn.PublicPartitions {
		fn.NoiseEpsilon = epsilon
		fn.NoiseDelta = delta
		return fn, nil
	}
	fn.NoiseEpsilon = epsilon / 2
	fn.PartitionSelectionEpsilon = epsilon - fn.NoiseEpsilon
	switch noiseKind {
	case noise.GaussianNoise:
		fn.NoiseDelta = delta / 2
	case noise.LaplaceNoise:
		fn.NoiseDelta = 0
	default:
		return nil, fmt.Errorf("unknown noise.Kind (%v) is specified. Please specify a valid noise", noiseKind)
	}
	fn.PartitionSelectionDelta = delta - fn.NoiseDelta
	return fn, nil
}

// newBoundedSumFloat64FnTemp returns a boundedSumFloat64Fn with the given budget and parameters.
//
// Uses the new privacy budget API where clients specify aggregation budget and partition selection budget separately.
func newBoundedSumFloat64FnTemp(aggregationEpsilon, aggregationDelta, partitionSelectionEpsilon, partitionSelectionDelta float64, preThreshold, maxPartitionsContributed int64, lower, upper float64, noiseKind noise.Kind, publicPartitions bool, testMode TestMode) (*boundedSumFloat64Fn, error) {
	if noiseKind != noise.GaussianNoise && noiseKind != noise.LaplaceNoise {
		return nil, fmt.Errorf("unknown noise.Kind (%v) is specified. Please specify a valid noise", noiseKind)
	}
	return &boundedSumFloat64Fn{
		NoiseEpsilon:              aggregationEpsilon,
		NoiseDelta:                aggregationDelta,
		PartitionSelectionEpsilon: partitionSelectionEpsilon,
		PartitionSelectionDelta:   partitionSelectionDelta,
		PreThreshold:              preThreshold,
		MaxPartitionsContributed:  maxPartitionsContributed,
		Lower:                     lower,
		Upper:                     upper,
		NoiseKind:                 noiseKind,
		PublicPartitions:          publicPartitions,
		TestMode:                  testMode,
	}, nil
}

func (fn *boundedSumFloat64Fn) Setup() {
	fn.noise = noise.ToNoise(fn.NoiseKind)
	if fn.TestMode.isEnabled() {
		fn.noise = noNoise{}
	}
}

func (fn *boundedSumFloat64Fn) CreateAccumulator() (boundedSumAccumFloat64, error) {
	if fn.TestMode == NoNoiseWithoutContributionBounding {
		fn.Lower = math.Inf(-1)
		fn.Upper = math.Inf(1)
	}
	var bs *dpagg.BoundedSumFloat64
	var err error
	bs, err = dpagg.NewBoundedSumFloat64(&dpagg.BoundedSumFloat64Options{
		Epsilon:                  fn.NoiseEpsilon,
		Delta:                    fn.NoiseDelta,
		MaxPartitionsContributed: fn.MaxPartitionsContributed,
		Lower:                    fn.Lower,
		Upper:                    fn.Upper,
		Noise:                    fn.noise,
	})
	if err != nil {
		return boundedSumAccumFloat64{}, err
	}
	accum := boundedSumAccumFloat64{BS: bs, PublicPartitions: fn.PublicPartitions}
	if !fn.PublicPartitions {
		accum.SP, err = dpagg.NewPreAggSelectPartition(&dpagg.PreAggSelectPartitionOptions{
			Epsilon:                  fn.PartitionSelectionEpsilon,
			Delta:                    fn.PartitionSelectionDelta,
			PreThreshold:             fn.PreThreshold,
			MaxPartitionsContributed: fn.MaxPartitionsContributed,
		})
	}
	return accum, err
}

func (fn *boundedSumFloat64Fn) AddInput(a boundedSumAccumFloat64, value float64) (boundedSumAccumFloat64, error) {
	var err error
	err = a.BS.Add(value)
	if err != nil {
		return a, err
	}
	if !fn.PublicPartitions {
		err = a.SP.Increment()
	}
	return a, err
}

func (fn *boundedSumFloat64Fn) MergeAccumulators(a, b boundedSumAccumFloat64) (boundedSumAccumFloat64, error) {
	var err error
	err = a.BS.Merge(b.BS)
	if err != nil {
		return a, err
	}
	if !fn.PublicPartitions {
		err = a.SP.Merge(b.SP)
	}
	return a, err
}

func (fn *boundedSumFloat64Fn) ExtractOutput(a boundedSumAccumFloat64) (*float64, error) {
	if fn.TestMode.isEnabled() {
		a.BS.Noise = noNoise{}
	}
	var err error
	shouldKeepPartition := fn.TestMode.isEnabled() || a.PublicPartitions // If in test mode or public partitions are specified, we always keep the partition.
	if !shouldKeepPartition {                                            // If not, we need to perform private partition selection.
		shouldKeepPartition, err = a.SP.ShouldKeepPartition()
		if err != nil {
			return nil, err
		}
	}

	if shouldKeepPartition {
		result, err := a.BS.Result()
		return &result, err
	}
	return nil, nil
}

// findDereferenceValueFn dereferences a *int64 to int64 or *float64 to float64.
func findDereferenceValueFn(kind reflect.Kind) (any, error) {
	switch kind {
	case reflect.Int64:
		return dereferenceValueToInt64, nil
	case reflect.Float64:
		return dereferenceValueToFloat64, nil
	default:
		return nil, fmt.Errorf("kind(%v) should be int64 or float64", kind)
	}
}

func dereferenceValueToInt64(key beam.W, value *int64) (k beam.W, v int64) {
	return key, *value
}

func dereferenceValueToFloat64(key beam.W, value *float64) (k beam.W, v float64) {
	return key, *value
}

func (fn *boundedSumFloat64Fn) String() string {
	return fmt.Sprintf("%#v", fn)
}

func findDropThresholdedPartitionsFn(kind reflect.Kind) (any, error) {
	switch kind {
	case reflect.Int64:
		return dropThresholdedPartitionsInt64, nil
	case reflect.Float64:
		return dropThresholdedPartitionsFloat64, nil
	default:
		return nil, fmt.Errorf("kind(%v) should be int64 or float64", kind)
	}
}

// dropThresholdedPartitionsInt64 drops thresholded int partitions, i.e. those
// that have nil r, by emitting only non-thresholded partitions.
func dropThresholdedPartitionsInt64(v beam.V, r *int64, emit func(beam.V, int64)) {
	if r != nil {
		emit(v, *r)
	}
}

// dropThresholdedPartitionsFloat64 drops thresholded float partitions, i.e. those
// that have nil r, by emitting only non-thresholded partitions.
func dropThresholdedPartitionsFloat64(v beam.V, r *float64, emit func(beam.V, float64)) {
	if r != nil {
		emit(v, *r)
	}
}

// dropThresholdedPartitionsFloat64Slice drops thresholded []float64 partitions, i.e.
// those that have nil r, by emitting only non-thresholded partitions.
func dropThresholdedPartitionsFloat64Slice(v beam.V, r []float64, emit func(beam.V, []float64)) {
	if r != nil {
		emit(v, r)
	}
}

func findClampNegativePartitionsFn(kind reflect.Kind) (any, error) {
	switch kind {
	case reflect.Int64:
		return clampNegativePartitionsInt64, nil
	case reflect.Float64:
		return clampNegativePartitionsFloat64, nil
	default:
		return nil, fmt.Errorf("kind(%v) should be int64 or float64", kind)
	}
}

// Clamp negative partitions to zero for int64 partitions, e.g., as a post aggregation step for Count.
func clampNegativePartitionsInt64(v beam.V, r int64) (beam.V, int64) {
	if r < 0 {
		return v, 0
	}
	return v, r
}

// Clamp negative partitions to zero for float64 partitions.
func clampNegativePartitionsFloat64(v beam.V, r float64) (beam.V, float64) {
	if r < 0 {
		return v, 0
	}
	return v, r
}

type dropValuesFn struct {
	Codec *kv.Codec
}

func (fn *dropValuesFn) Setup() {
	fn.Codec.Setup()
}

func (fn *dropValuesFn) ProcessElement(id beam.U, kv kv.Pair) (beam.U, beam.W, error) {
	k, _, err := fn.Codec.Decode(kv)
	return id, k, err
}

// encodeKVFn takes a PCollection<kv.Pair{ID,K}, codedV> as input, and returns a
// PCollection<ID, kv.Pair{K,V}>; where K and V have been coded, and ID has been
// decoded.
type encodeKVFn struct {
	InputPairCodec *kv.Codec // Codec for the input kv.Pair{ID,K}
}

func newEncodeKVFn(idkCodec *kv.Codec) *encodeKVFn {
	return &encodeKVFn{InputPairCodec: idkCodec}
}

func (fn *encodeKVFn) Setup() error {
	return fn.InputPairCodec.Setup()
}

func (fn *encodeKVFn) ProcessElement(pair kv.Pair, codedV []byte) (beam.W, kv.Pair, error) {
	id, _, err := fn.InputPairCodec.Decode(pair)
	return id, kv.Pair{pair.V, codedV}, err // pair.V is the K in PCollection<kv.Pair{ID,K}, codedV>
}

// encodeIDKFn takes a PCollection<ID,kv.Pair{K,V}> as input, and returns a
// PCollection<kv.Pair{ID,K},V>; where ID and K have been coded, and V has been
// decoded.
type encodeIDKFn struct {
	IDType         beam.EncodedType    // Type information of the privacy ID
	idEnc          beam.ElementEncoder // Encoder for privacy ID, set during Setup() according to IDType
	InputPairCodec *kv.Codec           // Codec for the input kv.Pair{K,V}
}

func newEncodeIDKFn(idType typex.FullType, kvCodec *kv.Codec) *encodeIDKFn {
	return &encodeIDKFn{
		IDType:         beam.EncodedType{idType.Type()},
		InputPairCodec: kvCodec,
	}
}

func (fn *encodeIDKFn) Setup() error {
	fn.idEnc = beam.NewElementEncoder(fn.IDType.T)
	return fn.InputPairCodec.Setup()
}

func (fn *encodeIDKFn) ProcessElement(id beam.W, pair kv.Pair) (kv.Pair, beam.V, error) {
	var idBuf bytes.Buffer
	if err := fn.idEnc.Encode(id, &idBuf); err != nil {
		return kv.Pair{}, nil, fmt.Errorf("pbeam.encodeIDKFn.ProcessElement: couldn't encode ID %v: %w", id, err)
	}
	_, v, err := fn.InputPairCodec.Decode(pair)
	return kv.Pair{idBuf.Bytes(), pair.K}, v, err
}

// decodeIDKFn is the reverse operation of encodeIDKFn. It takes a PCollection<kv.Pair{ID,K},V>
// as input, and returns a PCollection<ID, kv.Pair{K,V}>; where K and V has been coded, and ID
// has been decoded.
type decodeIDKFn struct {
	VType          beam.EncodedType    // Type information of the value V
	vEnc           beam.ElementEncoder // Encoder for privacy ID, set during Setup() according to VType
	InputPairCodec *kv.Codec           // Codec for the input kv.Pair{ID,K}
}

func newDecodeIDKFn(vType typex.FullType, idkCodec *kv.Codec) *decodeIDKFn {
	return &decodeIDKFn{
		VType:          beam.EncodedType{vType.Type()},
		InputPairCodec: idkCodec,
	}
}

func (fn *decodeIDKFn) Setup() error {
	fn.vEnc = beam.NewElementEncoder(fn.VType.T)
	return fn.InputPairCodec.Setup()
}

func (fn *decodeIDKFn) ProcessElement(pair kv.Pair, v beam.V) (beam.W, kv.Pair, error) {
	var vBuf bytes.Buffer
	if err := fn.vEnc.Encode(v, &vBuf); err != nil {
		return nil, kv.Pair{}, fmt.Errorf("pbeam.decodeIDKFn.ProcessElement: couldn't encode V %v: %w", v, err)
	}
	id, _, err := fn.InputPairCodec.Decode(pair)
	return id, kv.Pair{pair.V, vBuf.Bytes()}, err // pair.V is the K in PCollection<kv.Pair{ID,K},V>
}

// findConvertToFloat64Fn gets the correct conversion to float64 function.
func findConvertToFloat64Fn(t typex.FullType) (any, error) {
	switch t.Type().String() {
	case "int":
		return convertIntToFloat64, nil
	case "int8":
		return convertInt8ToFloat64, nil
	case "int16":
		return convertInt16ToFloat64, nil
	case "int32":
		return convertInt32ToFloat64, nil
	case "int64":
		return convertInt64ToFloat64, nil
	case "uint":
		return convertUintToFloat64, nil
	case "uint8":
		return convertUint8ToFloat64, nil
	case "uint16":
		return convertUint16ToFloat64, nil
	case "uint32":
		return convertUint32ToFloat64, nil
	case "uint64":
		return convertUint64ToFloat64, nil
	case "float32":
		return convertFloat32ToFloat64, nil
	case "float64":
		return convertFloat64ToFloat64, nil
	default:
		return nil, fmt.Errorf("unexpected value type of %v", t)
	}
}

func convertIntToFloat64(idk kv.Pair, i int) (kv.Pair, float64) {
	return idk, float64(i)
}

func convertInt8ToFloat64(idk kv.Pair, i int8) (kv.Pair, float64) {
	return idk, float64(i)
}

func convertInt16ToFloat64(idk kv.Pair, i int16) (kv.Pair, float64) {
	return idk, float64(i)
}

func convertInt32ToFloat64(idk kv.Pair, i int32) (kv.Pair, float64) {
	return idk, float64(i)
}

func convertInt64ToFloat64(idk kv.Pair, i int64) (kv.Pair, float64) {
	return idk, float64(i)
}

func convertUintToFloat64(idk kv.Pair, i uint) (kv.Pair, float64) {
	return idk, float64(i)
}

func convertUint8ToFloat64(idk kv.Pair, i uint8) (kv.Pair, float64) {
	return idk, float64(i)
}

func convertUint16ToFloat64(idk kv.Pair, i uint16) (kv.Pair, float64) {
	return idk, float64(i)
}

func convertUint32ToFloat64(idk kv.Pair, i uint32) (kv.Pair, float64) {
	return idk, float64(i)
}

func convertUint64ToFloat64(idk kv.Pair, i uint64) (kv.Pair, float64) {
	return idk, float64(i)
}

func convertFloat32ToFloat64(idk kv.Pair, f float32) (kv.Pair, float64) {
	return idk, float64(f)
}

func convertFloat64ToFloat64(idk kv.Pair, f float64) (kv.Pair, float64) {
	return idk, f
}

type expandValuesAccum struct {
	Values [][]byte
}

// expandValuesCombineFn converts a PCollection<K,V> to PCollection<K,[]V> where each value
// corresponding to the same key are collected in a slice. Resulting PCollection has a
// single slice for each key.
type expandValuesCombineFn struct {
	VType beam.EncodedType
	vEnc  beam.ElementEncoder
}

func newExpandValuesCombineFn(vType beam.EncodedType) *expandValuesCombineFn {
	return &expandValuesCombineFn{VType: vType}
}

func (fn *expandValuesCombineFn) Setup() {
	fn.vEnc = beam.NewElementEncoder(fn.VType.T)
}

func (fn *expandValuesCombineFn) CreateAccumulator() expandValuesAccum {
	return expandValuesAccum{Values: make([][]byte, 0)}
}

func (fn *expandValuesCombineFn) AddInput(a expandValuesAccum, value beam.V) (expandValuesAccum, error) {
	var vBuf bytes.Buffer
	if err := fn.vEnc.Encode(value, &vBuf); err != nil {
		return a, fmt.Errorf("pbeam.expandValuesCombineFn.AddInput: couldn't encode V %v: %w", value, err)
	}
	a.Values = append(a.Values, vBuf.Bytes())
	return a, nil
}

func (fn *expandValuesCombineFn) MergeAccumulators(a, b expandValuesAccum) expandValuesAccum {
	a.Values = append(a.Values, b.Values...)
	return a
}

func (fn *expandValuesCombineFn) ExtractOutput(a expandValuesAccum) [][]byte {
	return a.Values
}

type expandFloat64ValuesAccum struct {
	Values []float64
}

// expandFloat64ValuesCombineFn converts a PCollection<K,float64> to PCollection<K,[]float64>
// where each value corresponding to the same key are collected in a slice. Resulting
// PCollection has a single slice for each key.
type expandFloat64ValuesCombineFn struct{}

func (fn *expandFloat64ValuesCombineFn) CreateAccumulator() expandFloat64ValuesAccum {
	return expandFloat64ValuesAccum{Values: make([]float64, 0)}
}

func (fn *expandFloat64ValuesCombineFn) AddInput(a expandFloat64ValuesAccum, value float64) expandFloat64ValuesAccum {
	a.Values = append(a.Values, value)
	return a
}

func (fn *expandFloat64ValuesCombineFn) MergeAccumulators(a, b expandFloat64ValuesAccum) expandFloat64ValuesAccum {
	a.Values = append(a.Values, b.Values...)
	return a
}

func (fn *expandFloat64ValuesCombineFn) ExtractOutput(a expandFloat64ValuesAccum) []float64 {
	return a.Values
}

// checkPartitionSelectionEpsilon returns an error if the partitionSelectionEpsilon parameter of an aggregation is not valid.
// Epsilon is valid in the following cases:
//
//	epsilon == 0; if public partitions are used
//	0 < epsilon < +∞; otherwise
func checkPartitionSelectionEpsilon(epsilon float64, publicPartitions any) error {
	if publicPartitions != nil {
		if epsilon != 0 {
			return fmt.Errorf("PartitionSelectionEpsilon is %e, using public partitions requires setting PartitionSelectionEpsilon to 0", epsilon)
		}
		return nil
	}
	return checks.CheckEpsilonStrict(epsilon)
}

// checkDelta returns an error if the delta parameter of an aggregation is not valid. Delta
// is valid in the following cases:
//
//	delta == 0; when laplace noise with public partitions is used
//	0 < delta < 1; otherwise
func checkDelta(delta float64, noiseKind noise.Kind, publicPartitions any) error {
	if publicPartitions != nil && noiseKind == noise.LaplaceNoise {
		return checks.CheckNoDelta(delta)
	}
	return checks.CheckDeltaStrict(delta)
}

// checkAggregationDelta returns an error if the aggregationDelta parameter of an aggregation is not valid.
// Delta is valid in the following cases:
//
//	delta == 0; when laplace noise is used
//	0 < delta < 1; otherwise
func checkAggregationDelta(delta float64, noiseKind noise.Kind) error {
	if noiseKind == noise.LaplaceNoise {
		return checks.CheckNoDelta(delta)
	}
	return checks.CheckDeltaStrict(delta)
}

// checkPartitionSelectionDelta returns an error if the partitionSelectionDelta parameter of an aggregation is not valid.
// Delta is valid in the following cases:
//
//	delta == 0; if public partitions are used
//	0 < delta < 1; otherwise
func checkPartitionSelectionDelta(delta float64, publicPartitions any) error {
	if publicPartitions != nil {
		return checks.CheckNoDelta(delta)
	}
	return checks.CheckDeltaStrict(delta)
}

// checkMaxPartitionsContributed returns a maxPartitionsContributed parameter
// if it greater than zero, otherwise it fails.
func checkMaxPartitionsContributed(maxPartitionsContributed int64) error {
	if maxPartitionsContributed <= 0 {
		return fmt.Errorf("MaxPartitionsContributed must be set to a positive value, was %d instead", maxPartitionsContributed)
	}
	return nil
}
