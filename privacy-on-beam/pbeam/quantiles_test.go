//
// Copyright 2021 Google LLC
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

package pbeam

import (
	"reflect"
	"testing"

	"github.com/google/differential-privacy/go/v2/dpagg"
	"github.com/google/differential-privacy/go/v2/noise"
	"github.com/google/differential-privacy/privacy-on-beam/v2/pbeam/testutils"
	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/testing/ptest"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/transforms/stats"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestNewBoundedQuantilesFn(t *testing.T) {
	opts := []cmp.Option{
		cmpopts.EquateApprox(0, 1e-10),
		cmpopts.IgnoreUnexported(boundedQuantilesFn{}),
	}
	for _, tc := range []struct {
		desc      string
		noiseKind noise.Kind
		want      any
	}{
		{"Laplace noise kind", noise.LaplaceNoise,
			&boundedQuantilesFn{
				NoiseEpsilon:                 0.5,
				PartitionSelectionEpsilon:    0.5,
				NoiseDelta:                   0,
				PartitionSelectionDelta:      1e-5,
				MaxPartitionsContributed:     17,
				MaxContributionsPerPartition: 5,
				Lower:                        0,
				Upper:                        10,
				Ranks:                        []float64{0.1, 0.5, 0.9},
				NoiseKind:                    noise.LaplaceNoise,
			}},
		{"Gaussian noise kind", noise.GaussianNoise,
			&boundedQuantilesFn{
				NoiseEpsilon:                 0.5,
				PartitionSelectionEpsilon:    0.5,
				NoiseDelta:                   5e-6,
				PartitionSelectionDelta:      5e-6,
				MaxPartitionsContributed:     17,
				MaxContributionsPerPartition: 5,
				Lower:                        0,
				Upper:                        10,
				Ranks:                        []float64{0.1, 0.5, 0.9},
				NoiseKind:                    noise.GaussianNoise,
			}},
	} {
		got, err := newBoundedQuantilesFn(QuantilesParams{
			Epsilon:                      1,
			Delta:                        1e-5,
			MaxPartitionsContributed:     17,
			MaxContributionsPerPartition: 5,
			MinValue:                     0,
			MaxValue:                     10,
			Ranks:                        []float64{0.1, 0.5, 0.9},
		}, tc.noiseKind, false, Disabled, false)
		if err != nil {
			t.Fatalf("Couldn't get newBoundedQuantilesFn: %v", err)
		}
		if diff := cmp.Diff(tc.want, got, opts...); diff != "" {
			t.Errorf("newBoundedQuantilesFn: for %q (-want +got):\n%s", tc.desc, diff)
		}
	}
}

// The logic mirrors TestNewBoundedQuantilesFn, but with the new privacy budget API where
// clients specify aggregation budget and partition selection budget separately.
func TestNewBoundedQuantilesFnTemp(t *testing.T) {
	opts := []cmp.Option{
		cmpopts.EquateApprox(0, 1e-10),
		cmpopts.IgnoreUnexported(boundedQuantilesFn{}),
	}
	for _, tc := range []struct {
		desc                      string
		noiseKind                 noise.Kind
		aggregationEpsilon        float64
		aggregationDelta          float64
		partitionSelectionEpsilon float64
		partitionSelectionDelta   float64
		preThreshold              int64
		want                      *boundedQuantilesFn
	}{
		{"Laplace noise kind", noise.LaplaceNoise, 1.0, 0, 1.0, 1e-5, 0,
			&boundedQuantilesFn{
				NoiseEpsilon:                 1.0,
				NoiseDelta:                   0,
				PartitionSelectionEpsilon:    1.0,
				PartitionSelectionDelta:      1e-5,
				MaxPartitionsContributed:     17,
				MaxContributionsPerPartition: 5,
				Lower:                        0,
				Upper:                        10,
				Ranks:                        []float64{0.1, 0.5, 0.9},
				NoiseKind:                    noise.LaplaceNoise,
			}},
		{"Gaussian noise kind", noise.GaussianNoise, 1.0, 1e-5, 1.0, 1e-5, 0,
			&boundedQuantilesFn{
				NoiseEpsilon:                 1.0,
				NoiseDelta:                   1e-5,
				PartitionSelectionEpsilon:    1.0,
				PartitionSelectionDelta:      1e-5,
				MaxPartitionsContributed:     17,
				MaxContributionsPerPartition: 5,
				Lower:                        0,
				Upper:                        10,
				Ranks:                        []float64{0.1, 0.5, 0.9},
				NoiseKind:                    noise.GaussianNoise,
			}},
		{"PreThreshold set", noise.GaussianNoise, 1.0, 1e-5, 1.0, 1e-5, 10,
			&boundedQuantilesFn{
				NoiseEpsilon:                 1.0,
				NoiseDelta:                   1e-5,
				PartitionSelectionEpsilon:    1.0,
				PartitionSelectionDelta:      1e-5,
				PreThreshold:                 10,
				MaxPartitionsContributed:     17,
				MaxContributionsPerPartition: 5,
				Lower:                        0,
				Upper:                        10,
				Ranks:                        []float64{0.1, 0.5, 0.9},
				NoiseKind:                    noise.GaussianNoise,
			}},
	} {
		got, err := newBoundedQuantilesFnTemp(PrivacySpec{preThreshold: tc.preThreshold, testMode: Disabled},
			QuantilesParams{
				AggregationEpsilon:           tc.aggregationEpsilon,
				AggregationDelta:             tc.aggregationDelta,
				PartitionSelectionParams:     PartitionSelectionParams{tc.partitionSelectionEpsilon, tc.partitionSelectionDelta},
				MaxPartitionsContributed:     17,
				MaxContributionsPerPartition: 5,
				MinValue:                     0,
				MaxValue:                     10,
				Ranks:                        []float64{0.1, 0.5, 0.9},
			}, tc.noiseKind, false, false)
		if err != nil {
			t.Fatalf("Couldn't get newBoundedQuantilesFn: %v", err)
		}
		if diff := cmp.Diff(tc.want, got, opts...); diff != "" {
			t.Errorf("newBoundedQuantilesFnTemp: for %q (-want +got):\n%s", tc.desc, diff)
		}
	}
}

func TestBoundedQuantilesFnSetup(t *testing.T) {
	for _, tc := range []struct {
		desc      string
		noiseKind noise.Kind
		wantNoise any
	}{
		{"Laplace noise kind", noise.LaplaceNoise, noise.Laplace()},
		{"Gaussian noise kind", noise.GaussianNoise, noise.Gaussian()}} {
		got, err := newBoundedQuantilesFn(QuantilesParams{
			Epsilon:                      1,
			Delta:                        1e-5,
			MaxPartitionsContributed:     17,
			MaxContributionsPerPartition: 5,
			MinValue:                     0,
			MaxValue:                     10,
			Ranks:                        []float64{0.1, 0.5, 0.9},
		}, tc.noiseKind, false, Disabled, false)
		if err != nil {
			t.Fatalf("Couldn't get newBoundedQuantilesFn: %v", err)
		}
		got.Setup()
		if !cmp.Equal(tc.wantNoise, got.noise) {
			t.Errorf("Setup: for %s got %v, want %v", tc.desc, got.noise, tc.wantNoise)
		}
	}
}

func TestBoundedQuantilesFnAddInput(t *testing.T) {
	// δ=10⁻²³, ε=1e100 and l0Sensitivity=1 gives a threshold of =2.
	// Since ε=1e100, the noise is added with probability in the order of exp(-1e100).
	maxContributionsPerPartition := int64(1)
	maxPartitionsContributed := int64(1)
	epsilon := 1e100
	delta := 1e-23
	lower := 0.0
	upper := 5.0
	ranks := []float64{0.25, 0.75}
	// ε is split in two for noise and for partition selection, so we use 2*ε to get a Laplace noise with ε.
	fn, err := newBoundedQuantilesFn(QuantilesParams{
		Epsilon:                      2 * epsilon,
		Delta:                        delta,
		MaxPartitionsContributed:     maxPartitionsContributed,
		MaxContributionsPerPartition: maxContributionsPerPartition,
		MinValue:                     lower,
		MaxValue:                     upper,
		Ranks:                        ranks,
	}, noise.LaplaceNoise, false, Disabled, false)
	if err != nil {
		t.Fatalf("Couldn't get newBoundedQuantilesFn: %v", err)
	}
	fn.Setup()

	accum, err := fn.CreateAccumulator()
	if err != nil {
		t.Fatalf("Couldn't create accum: %v", err)
	}
	for i := 0; i < 100; i++ {
		fn.AddInput(accum, []float64{1.0})
		fn.AddInput(accum, []float64{4.0})
	}

	got, err := fn.ExtractOutput(accum)
	if err != nil {
		t.Fatalf("Couldn't extract output: %v", err)
	}
	tolerance := testutils.QuantilesTolerance(lower, upper)
	want := []float64{1.0, 4.0} // Corresponding to ranks 0.25 and 0.75, respectively.
	for i, rank := range ranks {
		if !cmp.Equal(want[i], got[i], cmpopts.EquateApprox(0, tolerance)) {
			t.Errorf("AddInput: for rank: %f values got: %f, want %f", rank, got[i], want[i])
		}
	}
}

func TestBoundedQuantilesFnMergeAccumulators(t *testing.T) {
	// δ=10⁻²³, ε=1e100 and l0Sensitivity=1 gives a threshold of =2.
	// Since ε=1e100, the noise is added with probability in the order of exp(-1e100).
	maxContributionsPerPartition := int64(1)
	maxPartitionsContributed := int64(1)
	epsilon := 1e100
	delta := 1e-23
	lower := 0.0
	upper := 5.0
	ranks := []float64{0.25, 0.75}
	// ε is split in two for noise and for partition selection, so we use 2*ε to get a Laplace noise with ε.
	fn, err := newBoundedQuantilesFn(QuantilesParams{
		Epsilon:                      2 * epsilon,
		Delta:                        delta,
		MaxPartitionsContributed:     maxPartitionsContributed,
		MaxContributionsPerPartition: maxContributionsPerPartition,
		MinValue:                     lower,
		MaxValue:                     upper,
		Ranks:                        ranks,
	}, noise.LaplaceNoise, false, Disabled, false)
	if err != nil {
		t.Fatalf("Couldn't get newBoundedQuantilesFn: %v", err)
	}
	fn.Setup()

	accum1, err := fn.CreateAccumulator()
	if err != nil {
		t.Fatalf("Couldn't create accum1: %v", err)
	}
	accum2, err := fn.CreateAccumulator()
	if err != nil {
		t.Fatalf("Couldn't create accum2: %v", err)
	}
	for i := 0; i < 100; i++ {
		fn.AddInput(accum1, []float64{1.0})
		fn.AddInput(accum2, []float64{4.0})
	}
	fn.MergeAccumulators(accum1, accum2)

	got, err := fn.ExtractOutput(accum1)
	if err != nil {
		t.Fatalf("Couldn't extract output: %v", err)
	}
	tolerance := testutils.QuantilesTolerance(lower, upper)
	want := []float64{1.0, 4.0} // Corresponding to ranks 0.25 and 0.75, respectively.
	for i, rank := range ranks {
		if !cmp.Equal(want[i], got[i], cmpopts.EquateApprox(0, tolerance)) {
			t.Errorf("MergeAccumulators: for rank: %f values got: %f, want %f", rank, got[i], want[i])
		}
	}
}

func TestBoundedQuantilesFnExtractOutputReturnsNilForSmallPartitions(t *testing.T) {
	for _, tc := range []struct {
		desc                     string
		inputSize                int
		datapointsPerPrivacyUnit int
	}{
		// It's a special case for partition selection in which the algorithm should always eliminate the partition.
		{"Empty input", 0, 0},
		{"Input with 1 privacy unit with 1 contribution", 1, 1},
	} {
		// The choice of ε=1e100, δ=10⁻²³, and l0Sensitivity=1 gives a threshold of =2.
		// ε is split in two for noise and for partition selection, so we use 2*ε to get a Laplace noise with ε.
		fn, err := newBoundedQuantilesFn(QuantilesParams{
			Epsilon:                      2 * 1e100,
			Delta:                        1e-23,
			MaxPartitionsContributed:     1,
			MaxContributionsPerPartition: 1,
			MinValue:                     0,
			MaxValue:                     10,
			Ranks:                        []float64{0.5},
		}, noise.LaplaceNoise, false, Disabled, false)
		if err != nil {
			t.Fatalf("Couldn't get newBoundedQuantilesFn: %v", err)
		}
		fn.Setup()
		accum, err := fn.CreateAccumulator()
		if err != nil {
			t.Fatalf("Couldn't create accum: %v", err)
		}
		for i := 0; i < tc.inputSize; i++ {
			values := make([]float64, tc.datapointsPerPrivacyUnit)
			for i := 0; i < tc.datapointsPerPrivacyUnit; i++ {
				values[i] = 1.0
			}
			fn.AddInput(accum, values)
		}

		got, err := fn.ExtractOutput(accum)
		if err != nil {
			t.Fatalf("Couldn't extract output: %v", err)
		}

		// Should return nil output for small partitions.
		if got != nil {
			t.Errorf("ExtractOutput: for %s got: %f, want nil", tc.desc, got)
		}
	}
}

func TestBoundedQuantilesFnWithPartitionsExtractOutputDoesNotReturnNilForSmallPartitions(t *testing.T) {
	for _, tc := range []struct {
		desc                     string
		inputSize                int
		datapointsPerPrivacyUnit int
	}{
		{"Empty input", 0, 0},
		{"Input with 1 privacy unit with 1 contribution", 1, 1},
	} {
		fn, err := newBoundedQuantilesFn(QuantilesParams{
			Epsilon:                      1e100,
			MaxPartitionsContributed:     1,
			MaxContributionsPerPartition: 1,
			MinValue:                     0,
			MaxValue:                     10,
			Ranks:                        []float64{0.5},
		}, noise.LaplaceNoise, true, Disabled, false)
		if err != nil {
			t.Fatalf("Couldn't get newBoundedQuantilesFn: %v", err)
		}
		fn.Setup()
		accum, err := fn.CreateAccumulator()
		if err != nil {
			t.Fatalf("Couldn't create accum: %v", err)
		}
		for i := 0; i < tc.inputSize; i++ {
			values := make([]float64, tc.datapointsPerPrivacyUnit)
			for i := 0; i < tc.datapointsPerPrivacyUnit; i++ {
				values[i] = 1.0
			}
			fn.AddInput(accum, values)
		}

		got, err := fn.ExtractOutput(accum)
		if err != nil {
			t.Fatalf("Couldn't extract output: %v", err)
		}

		// Should not return nil output for small partitions in the case of public partitions.
		if got == nil {
			t.Errorf("ExtractOutput for %s thresholded with public partitions when it shouldn't", tc.desc)
		}
	}
}

// Checks that QuantilesPerKey adds noise to its output.
func TestQuantilesPerKeyAddsNoise(t *testing.T) {
	for _, tc := range []struct {
		name      string
		noiseKind NoiseKind
		// Differential privacy params used
		epsilon float64
		delta   float64
	}{
		{
			name:      "Gaussian",
			noiseKind: GaussianNoise{},
			epsilon:   0.1,  // It is split in two: 0.05 for the noise and 0.05 for the partition selection.
			delta:     2e-3, // It is split in two: 1e-3 for the noise and 1e-3 for the partition selection.
		},
		{
			name:      "Laplace",
			noiseKind: LaplaceNoise{},
			epsilon:   0.1, // It is split in two: 0.05 for the noise and 0.05 for the partition selection.
			delta:     1e-3,
		},
	} {
		ranks := []float64{0.50}
		// triples contains {1,0,0.5}, {2,0,1}, …, {200,0,100}.
		var triples []testutils.TripleWithFloatValue
		for i := 0; i < 200; i++ {
			triples = append(triples, testutils.TripleWithFloatValue{ID: i, Partition: 0, Value: float32(i) / 2})
		}
		// ε=0.1, δ=10⁻³ and l0Sensitivity=1 gives a threshold of 132.
		// We have 200 privacy IDs, so we will keep the partition.
		p, s, col := ptest.CreateList(triples)
		col = beam.ParDo(s, testutils.ExtractIDFromTripleWithFloatValue, col)

		// Use twice epsilon & delta because we compute Quantiles twice.
		pcol := MakePrivate(s, col, NewPrivacySpec(2*tc.epsilon, 2*tc.delta))
		pcol = ParDo(s, testutils.TripleWithFloatValueToKV, pcol)
		got1 := QuantilesPerKey(s, pcol, QuantilesParams{
			Epsilon:                      tc.epsilon,
			Delta:                        tc.delta,
			MaxPartitionsContributed:     1,
			MaxContributionsPerPartition: 1,
			MinValue:                     0.0,
			MaxValue:                     2.0,
			NoiseKind:                    tc.noiseKind,
			Ranks:                        ranks,
		})
		got2 := QuantilesPerKey(s, pcol, QuantilesParams{
			Epsilon:                      tc.epsilon,
			Delta:                        tc.delta,
			MaxPartitionsContributed:     1,
			MaxContributionsPerPartition: 1,
			MinValue:                     0.0,
			MaxValue:                     2.0,
			NoiseKind:                    tc.noiseKind,
			Ranks:                        ranks,
		})
		got1 = beam.ParDo(s, testutils.DereferenceFloat64Slice, got1)
		got2 = beam.ParDo(s, testutils.DereferenceFloat64Slice, got2)

		if err := testutils.NotEqualsFloat64(s, got1, got2); err != nil {
			t.Fatalf("NotEqualsFloat64: got error %v", err)
		}
		if err := ptest.Run(p); err != nil {
			t.Errorf("QuantilesPerKey didn't add any noise with float inputs and %s Noise: %v", tc.name, err)
		}
	}
}

// Checks that QuantilesPerKey with partitions adds noise to its output.
func TestQuantilesWithPartitionsPerKeyAddsNoise(t *testing.T) {
	for _, tc := range []struct {
		desc      string
		noiseKind NoiseKind
		epsilon   float64
		delta     float64
		inMemory  bool
	}{
		{
			desc:      "as PCollection w/ Gaussian",
			noiseKind: GaussianNoise{},
			epsilon:   0.05,
			delta:     1e-5,
			inMemory:  false,
		},
		{
			desc:      "as slice w/ Gaussian",
			noiseKind: GaussianNoise{},
			epsilon:   0.05,
			delta:     1e-5,
			inMemory:  true,
		},
		{
			desc:      "as PCollection w/ Laplace",
			noiseKind: LaplaceNoise{},
			epsilon:   0.05,
			delta:     0.0, // It is 0 because partitions are public and we are using Laplace noise.
			inMemory:  false,
		},
		{
			desc:      "as slice w/ Laplace",
			noiseKind: LaplaceNoise{},
			epsilon:   0.05,
			delta:     0.0, // It is 0 because partitions are public and we are using Laplace noise.
			inMemory:  true,
		},
	} {
		ranks := []float64{0.50}
		// triples contains {1,0,1}, {2,0,2}, …, {100,0,100}.
		var triples []testutils.TripleWithFloatValue
		for i := 0; i < 100; i++ {
			triples = append(triples, testutils.TripleWithFloatValue{ID: i, Partition: 0, Value: float32(i)})
		}
		publicPartitionsSlice := []int{0}
		p, s, col := ptest.CreateList(triples)
		var publicPartitions any
		if tc.inMemory {
			publicPartitions = publicPartitionsSlice
		} else {
			publicPartitions = beam.CreateList(s, publicPartitionsSlice)
		}

		col = beam.ParDo(s, testutils.ExtractIDFromTripleWithFloatValue, col)

		// Use twice epsilon & delta because we compute Quantiles twice.
		pcol := MakePrivate(s, col, NewPrivacySpec(2*tc.epsilon, 2*tc.delta))
		pcol = ParDo(s, testutils.TripleWithFloatValueToKV, pcol)
		quantilesParams := QuantilesParams{
			Epsilon:                      tc.epsilon,
			Delta:                        tc.delta,
			MaxPartitionsContributed:     100,
			MaxContributionsPerPartition: 100,
			MinValue:                     0.0,
			MaxValue:                     100.0,
			NoiseKind:                    tc.noiseKind,
			Ranks:                        ranks,
			PublicPartitions:             publicPartitions,
		}
		got1 := QuantilesPerKey(s, pcol, quantilesParams)
		got2 := QuantilesPerKey(s, pcol, quantilesParams)
		got1 = beam.ParDo(s, testutils.DereferenceFloat64Slice, got1)
		got2 = beam.ParDo(s, testutils.DereferenceFloat64Slice, got2)

		if err := testutils.NotEqualsFloat64(s, got1, got2); err != nil {
			t.Fatalf("NotEqualsFloat64: got error %v", err)
		}
		if err := ptest.Run(p); err != nil {
			t.Errorf("QuantilesPerKey with partitions %s didn't add any noise: %v", tc.desc, err)
		}
	}
}

// Checks that QuantilesPerKey returns a correct answer.
func TestQuantilesPerKeyNoNoise(t *testing.T) {
	// Arrange
	triples := testutils.ConcatenateTriplesWithFloatValue(
		testutils.MakeTripleWithFloatValue(100, 0, 1.0),
		testutils.MakeTripleWithFloatValue(100, 0, 4.0))

	wantMetric := []testutils.PairIF64Slice{
		{0, []float64{1.0, 1.0, 4.0, 4.0}},
	}
	p, s, col, want := ptest.CreateList2(triples, wantMetric)
	col = beam.ParDo(s, testutils.ExtractIDFromTripleWithFloatValue, col)

	// ε=900, δ=10⁻²⁰⁰ and l0Sensitivity=1 gives a threshold of =2.
	epsilon := 900.0
	delta := 1e-200
	lower := 0.0
	upper := 5.0
	ranks := []float64{0.00, 0.25, 0.75, 1.00}

	// Act
	// ε is split in two for noise and for partition selection, so we use 2*ε to get a Laplace noise with ε.
	pcol := MakePrivate(s, col, NewPrivacySpec(2*epsilon, delta))
	pcol = ParDo(s, testutils.TripleWithFloatValueToKV, pcol)
	got := QuantilesPerKey(s, pcol, QuantilesParams{
		MaxPartitionsContributed:     1,
		MaxContributionsPerPartition: 2,
		MinValue:                     lower,
		MaxValue:                     upper,
		Ranks:                        ranks,
	})

	// Assert
	want = beam.ParDo(s, testutils.PairIF64SliceToKV, want)
	if err := testutils.ApproxEqualsKVFloat64Slice(s, got, want, testutils.QuantilesTolerance(lower, upper)); err != nil {
		t.Fatalf("ApproxEqualsKVFloat64Slice: got error %v", err)
	}
	if err := ptest.Run(p); err != nil {
		t.Errorf("QuantilesPerKey did not return approximate quantile: %v", err)
	}
}

// Checks that QuantilesPerKey with partitions returns a correct answer.
func TestQuantilesPerKeyWithPartitionsNoNoise(t *testing.T) {
	// We have two test cases, one for public partitions as a PCollection and one for public partitions as a slice (i.e., in-memory).
	for _, tc := range []struct {
		inMemory bool
	}{
		{true},
		{false},
	} {
		// Arrange
		triples := testutils.ConcatenateTriplesWithFloatValue(
			testutils.MakeTripleWithFloatValue(100, 0, 1.0),
			testutils.MakeTripleWithFloatValue(100, 0, 4.0))

		wantMetric := []testutils.PairIF64Slice{
			{0, []float64{1.0, 1.0, 4.0, 4.0}},
		}
		p, s, col, want := ptest.CreateList2(triples, wantMetric)

		epsilon := 900.0
		delta := 0.0
		lower := 0.0
		upper := 5.0
		ranks := []float64{0.00, 0.25, 0.75, 1.00}
		publicPartitionsSlice := []int{0}
		var publicPartitions any
		if tc.inMemory {
			publicPartitions = publicPartitionsSlice
		} else {
			publicPartitions = beam.CreateList(s, publicPartitionsSlice)
		}

		col = beam.ParDo(s, testutils.ExtractIDFromTripleWithFloatValue, col)

		// Act
		pcol := MakePrivate(s, col, NewPrivacySpec(epsilon, delta))
		pcol = ParDo(s, testutils.TripleWithFloatValueToKV, pcol)
		quantilesParams := QuantilesParams{
			MaxPartitionsContributed:     1,
			MaxContributionsPerPartition: 2,
			MinValue:                     lower,
			MaxValue:                     upper,
			Ranks:                        ranks,
			PublicPartitions:             publicPartitions,
		}
		got := QuantilesPerKey(s, pcol, quantilesParams)

		// Assert
		want = beam.ParDo(s, testutils.PairIF64SliceToKV, want)
		if err := testutils.ApproxEqualsKVFloat64Slice(s, got, want, testutils.QuantilesTolerance(lower, upper)); err != nil {
			t.Fatalf("ApproxEqualsKVFloat64Slice in-memory=%t: got error %v", tc.inMemory, err)
		}
		if err := ptest.Run(p); err != nil {
			t.Errorf("QuantilesPerKey with partitions in-memory=%t did not return approximate quantile: %v", tc.inMemory, err)
		}
	}
}

// Checks that QuantilesPerKey with partitions adds public partitions not found in
// the data and drops non-public partitions.
func TestQuantilesPerKeyWithPartitionsAppliesPublicPartitions(t *testing.T) {
	// We have two test cases, one for public partitions as a PCollection and one for public partitions as a slice (i.e., in-memory).
	for _, tc := range []struct {
		inMemory bool
	}{
		{true},
		{false},
	} {
		triples := testutils.ConcatenateTriplesWithFloatValue(
			testutils.MakeTripleWithFloatValue(1, 0, 1.0),
			testutils.MakeTripleWithFloatValueStartingFromKey(1, 1, 1, 1.0),
			testutils.MakeTripleWithFloatValueStartingFromKey(2, 1, 2, 1.0))
		p, s, col := ptest.CreateList(triples)

		epsilon := 900.0
		delta := 0.0
		lower := 0.0
		upper := 5.0
		ranks := []float64{0.00, 0.25, 0.75, 1.00}
		publicPartitionsSlice := []int{2, 3, 4, 5}
		var publicPartitions any
		if tc.inMemory {
			publicPartitions = publicPartitionsSlice
		} else {
			publicPartitions = beam.CreateList(s, publicPartitionsSlice)
		}

		col = beam.ParDo(s, testutils.ExtractIDFromTripleWithFloatValue, col)

		pcol := MakePrivate(s, col, NewPrivacySpec(epsilon, delta))
		pcol = ParDo(s, testutils.TripleWithFloatValueToKV, pcol)
		quantilesParams := QuantilesParams{
			MaxPartitionsContributed:     1,
			MaxContributionsPerPartition: 2,
			MinValue:                     lower,
			MaxValue:                     upper,
			Ranks:                        ranks,
			PublicPartitions:             publicPartitions,
		}
		got := QuantilesPerKey(s, pcol, quantilesParams)
		got = beam.DropKey(s, got) // We are only interested in the number of partitions kept

		// There are 4 public partitions, so we expect 4 partitions in the output.
		// If we didn't drop non-public partitions (partitions "0" and "1"), we would have
		// 6 (if we still added empty public partitions) or 3 (if we also didn't add empty
		// public partitions) partitions in the output.
		// Similarly, if we didn't add empty public partitions (partitions "3", "4", "5"),
		// we would have 1 (if we still dropped non-public partitions) or 3 (if we also
		// didn't drop non-public partitions) partitions in the output.
		wantPartitions := 4
		testutils.CheckNumPartitions(s, got, wantPartitions)
		if err := ptest.Run(p); err != nil {
			t.Errorf("QuantilesPerKey with partitions in-memory=%t did not apply public partitions: %v", tc.inMemory, err)
		}
	}
}

var quantilesPartitionSelectionTestCases = []struct {
	name                string
	noiseKind           NoiseKind
	epsilon             float64
	delta               float64
	numPartitions       int
	entriesPerPartition int
}{
	{
		name:      "Gaussian",
		noiseKind: GaussianNoise{},
		// After splitting the (ε, δ) budget between the noise and partition
		// selection portions of the privacy algorithm, this results in a ε=1,
		// δ=0.3 partition selection budget.
		epsilon: 2,
		delta:   0.6,
		// entriesPerPartition=1 yields a 30% chance of emitting any particular partition
		// (since δ_emit=0.3).
		entriesPerPartition: 1,
		// 143 distinct partitions implies that some (but not all) partitions are
		// emitted with high probability (at least 1 - 1e-20).
		numPartitions: 143,
	},
	{
		name:      "Laplace",
		noiseKind: LaplaceNoise{},
		// After splitting the (ε, δ) budget between the noise and partition
		// selection portions of the privacy algorithm, this results in the
		// partition selection portion of the budget being ε_selectPartition=1,
		// δ_selectPartition=0.3.
		epsilon: 2,
		delta:   0.3,
		// entriesPerPartition=1 yields a 30% chance of emitting any particular partition
		// (since δ_emit=0.3).
		entriesPerPartition: 1,
		numPartitions:       143,
	},
}

// Checks that QuantilesPerKey applies partition selection.
func TestQuantilesPartitionSelection(t *testing.T) {
	for _, tc := range quantilesPartitionSelectionTestCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verify that entriesPerPartition is sensical.
			if tc.entriesPerPartition <= 0 {
				t.Fatalf("Invalid test case: entriesPerPartition must be positive. Got: %d", tc.entriesPerPartition)
			}

			// Build up {ID, Partition, Value} pairs such that for each of the tc.numPartitions partitions,
			// tc.entriesPerPartition privacy units contribute a single value:
			//    {0, 0, 1}, {1, 0, 1}, …, {entriesPerPartition-1, 0, 1}
			//    {entriesPerPartition, 1, 1}, {entriesPerPartition+1, 1, 1}, …, {entriesPerPartition+entriesPerPartition-1, 1, 1}
			//    …
			//    {entriesPerPartition*(numPartitions-1), numPartitions-1, 1}, …, {entriesPerPartition*numPartitions-1, numPartitions-1, 1}
			var (
				triples []testutils.TripleWithFloatValue
				kOffset = 0
			)
			for i := 0; i < tc.numPartitions; i++ {
				for j := 0; j < tc.entriesPerPartition; j++ {
					triples = append(triples, testutils.TripleWithFloatValue{ID: kOffset + j, Partition: i, Value: 1.0})
				}
				kOffset += tc.entriesPerPartition
			}
			p, s, col := ptest.CreateList(triples)
			col = beam.ParDo(s, testutils.ExtractIDFromTripleWithFloatValue, col)

			// Run QuantilesPerKey on triples
			ranks := []float64{0.00, 0.25, 0.75, 1.00}
			pcol := MakePrivate(s, col, NewPrivacySpec(tc.epsilon, tc.delta))
			pcol = ParDo(s, testutils.TripleWithFloatValueToKV, pcol)
			got := QuantilesPerKey(s, pcol, QuantilesParams{
				MinValue:                     0.0,
				MaxValue:                     1.0,
				MaxContributionsPerPartition: int64(tc.entriesPerPartition),
				MaxPartitionsContributed:     1,
				NoiseKind:                    tc.noiseKind,
				Ranks:                        ranks,
			})
			got = beam.ParDo(s, testutils.KVToPairIF64Slice, got)

			// Validate that partition selection is applied (i.e., some emitted and some dropped).
			testutils.CheckSomePartitionsAreDropped(s, got, tc.numPartitions)
			if err := ptest.Run(p); err != nil {
				t.Errorf("%v", err)
			}
		})
	}
}

// Checks that QuantilePerKey does cross-partition contribution bounding correctly.
func TestQuantilesPerKeyCrossPartitionContributionBounding(t *testing.T) {
	// 100 distinct privacy IDs contribute 0.0 to partition 0 and another 100 distinct privacy
	// IDs contribute 0.0 to partition 1. Then, another 100 privacy IDs (different from
	// these 200 privacy IDs) contributes "1.0"s to both partition 0 and partition 1.
	// MaxPartitionsContributed is 1, so a total of 100 "1.0" contributions will be kept across
	// both partitions. Depending on how contributions are kept, rank=0.60 of both partitions is
	// either both 0.0 or one is 1.0 and the other 0.0. Either way, the sum of rank=0.60 of both
	// partitions should be less than or equal to 1.0. (as opposed to 2.0 if no contribution bounding
	// takes place, since rank=0.60 will be 1.0 for both partitions in that case).
	triples := testutils.ConcatenateTriplesWithFloatValue(
		testutils.MakeTripleWithFloatValue(100, 0, 0.0),
		testutils.MakeTripleWithFloatValueStartingFromKey(100, 100, 1, 0.0),
		testutils.MakeTripleWithFloatValueStartingFromKey(200, 100, 0, 1.0),
		testutils.MakeTripleWithFloatValueStartingFromKey(200, 100, 1, 1.0),
	)
	lower := 0.0
	upper := 5.0
	wantMetric := []testutils.PairIF64{
		{0, 1.0 + testutils.QuantilesTolerance(lower, upper)}, // To account for noise.
	}
	p, s, col, want := ptest.CreateList2(triples, wantMetric)
	col = beam.ParDo(s, testutils.ExtractIDFromTripleWithFloatValue, col)

	// ε=900, δ=10⁻²⁰⁰ and l0Sensitivity=1 gives a threshold of =2.
	epsilon := 900.0
	delta := 1e-200
	ranks := []float64{0.60}

	// ε is split in two for noise and for partition selection, so we use 2*ε to get a Laplace noise with ε.
	pcol := MakePrivate(s, col, NewPrivacySpec(2*epsilon, delta))
	pcol = ParDo(s, testutils.TripleWithFloatValueToKV, pcol)
	got := QuantilesPerKey(s, pcol, QuantilesParams{
		MaxPartitionsContributed:     1,
		MaxContributionsPerPartition: 1,
		MinValue:                     lower,
		MaxValue:                     upper,
		Ranks:                        ranks,
	})
	got = beam.ParDo(s, testutils.DereferenceFloat64Slice, got)
	maxs := beam.DropKey(s, got)
	maxOverPartitions := stats.Sum(s, maxs)
	got = beam.AddFixedKey(s, maxOverPartitions) // Adds a fixed key of 0.

	want = beam.ParDo(s, testutils.PairIF64ToKV, want)
	if err := testutils.LessThanOrEqualToKVFloat64(s, got, want); err != nil {
		t.Fatalf("LessThanOrEqualToKVFloat64: got error %v", err)
	}
	if err := ptest.Run(p); err != nil {
		t.Errorf("QuantilesPerKey did not bound cross-partition contributions correctly: %v", err)
	}
}

// Checks that QuantilePerKey with partitions does cross-partition contribution bounding correctly.
func TestQuantilesPerKeyWithPartitionsCrossPartitionContributionBounding(t *testing.T) {
	// We have two test cases, one for public partitions as a PCollection and one for public partitions as a slice (i.e., in-memory).
	for _, tc := range []struct {
		inMemory bool
	}{
		{true},
		{false},
	} {
		// 100 distinct privacy IDs contribute 0.0 to partition 0 and another 100 distinct privacy
		// IDs contribute 0.0 to partition 1. Then, another 100 privacy IDs (different from
		// these 200 privacy IDs) contributes "1.0"s to both partition 0 and partition 1.
		// MaxPartitionsContributed is 1, so a total of 100 "1.0" contributions will be kept across
		// both partitions. Depending on how contributions are kept, rank=0.60 of both partitions is
		// either both 0.0 or one is 1.0 and the other 0.0. Either way, the sum of rank=0.60 of both
		// partitions should be less than or equal to 1.0. (as opposed to 2.0 if no contribution bounding
		// takes place, since rank=0.60 will be 1.0 for both partitions in that case).
		triples := testutils.ConcatenateTriplesWithFloatValue(
			testutils.MakeTripleWithFloatValue(100, 0, 0.0),
			testutils.MakeTripleWithFloatValueStartingFromKey(100, 100, 1, 0.0),
			testutils.MakeTripleWithFloatValueStartingFromKey(200, 100, 0, 1.0),
			testutils.MakeTripleWithFloatValueStartingFromKey(200, 100, 1, 1.0),
		)
		lower := 0.0
		upper := 5.0
		wantMetric := []testutils.PairIF64{
			{0, 1.0 + testutils.QuantilesTolerance(lower, upper)}, // To account for noise.
		}
		p, s, col, want := ptest.CreateList2(triples, wantMetric)

		epsilon := 900.0
		delta := 0.0
		ranks := []float64{0.60}
		publicPartitionsSlice := []int{0, 1}
		var publicPartitions any
		if tc.inMemory {
			publicPartitions = publicPartitionsSlice
		} else {
			publicPartitions = beam.CreateList(s, publicPartitionsSlice)
		}

		col = beam.ParDo(s, testutils.ExtractIDFromTripleWithFloatValue, col)

		pcol := MakePrivate(s, col, NewPrivacySpec(epsilon, delta))
		pcol = ParDo(s, testutils.TripleWithFloatValueToKV, pcol)
		quantilesParams := QuantilesParams{
			MaxPartitionsContributed:     1,
			MaxContributionsPerPartition: 1,
			MinValue:                     lower,
			MaxValue:                     upper,
			Ranks:                        ranks,
			PublicPartitions:             publicPartitions,
		}
		got := QuantilesPerKey(s, pcol, quantilesParams)
		got = beam.ParDo(s, testutils.DereferenceFloat64Slice, got)
		maxs := beam.DropKey(s, got)
		maxOverPartitions := stats.Sum(s, maxs)
		got = beam.AddFixedKey(s, maxOverPartitions) // Adds a fixed key of 0.

		want = beam.ParDo(s, testutils.PairIF64ToKV, want)
		if err := testutils.LessThanOrEqualToKVFloat64(s, got, want); err != nil {
			t.Fatalf("LessThanOrEqualToKVFloat64 in-memory=%t: got error %v", tc.inMemory, err)
		}
		if err := ptest.Run(p); err != nil {
			t.Errorf("QuantilesPerKey with partitions in-memory=%t did not bound cross-partition contributions correctly: %v", tc.inMemory, err)
		}
	}
}

// Checks that QuantilePerKey does per-partition contribution bounding correctly.
func TestQuantilesPerKeyPerPartitionContributionBounding(t *testing.T) {
	// 100 distinct privacy IDs contribute 0.0 and another 100 distinct privacy IDs
	// contribute 1.0 twice. MaxPartitionsContributed is 1, so only half of these
	// contributions will be kept and we expect equal number of 0.0's and 1.0s.
	triples := testutils.ConcatenateTriplesWithFloatValue(
		testutils.MakeTripleWithFloatValue(100, 0, 0.0),
		testutils.MakeTripleWithFloatValueStartingFromKey(100, 100, 0, 1.0),
		testutils.MakeTripleWithFloatValueStartingFromKey(100, 100, 0, 1.0))

	wantMetric := []testutils.PairIF64Slice{
		{0, []float64{0.0, 1.0}},
	}
	p, s, col, want := ptest.CreateList2(triples, wantMetric)
	col = beam.ParDo(s, testutils.ExtractIDFromTripleWithFloatValue, col)

	// ε=900, δ=10⁻²⁰⁰ and l0Sensitivity=1 gives a threshold of =2.
	epsilon := 900.0
	delta := 1e-200
	lower := 0.0
	upper := 5.0
	ranks := []float64{0.49, 0.51}

	// ε is split in two for noise and for partition selection, so we use 2*ε to get a Laplace noise with ε.
	pcol := MakePrivate(s, col, NewPrivacySpec(2*epsilon, delta))
	pcol = ParDo(s, testutils.TripleWithFloatValueToKV, pcol)
	got := QuantilesPerKey(s, pcol, QuantilesParams{
		MaxPartitionsContributed:     1,
		MaxContributionsPerPartition: 1,
		MinValue:                     lower,
		MaxValue:                     upper,
		Ranks:                        ranks,
	})

	want = beam.ParDo(s, testutils.PairIF64SliceToKV, want)
	if err := testutils.ApproxEqualsKVFloat64Slice(s, got, want, testutils.QuantilesTolerance(lower, upper)); err != nil {
		t.Fatalf("ApproxEqualsKVFloat64Slice: got error %v", err)
	}
	if err := ptest.Run(p); err != nil {
		t.Errorf("QuantilesPerKey did not bound cross-partition contributions correctly: %v", err)
	}
}

// Checks that QuantilePerKey with partitions does per-partition contribution bounding correctly.
func TestQuantilesPerKeyWithPartitionsPerPartitionContributionBounding(t *testing.T) {
	// We have two test cases, one for public partitions as a PCollection and one for public partitions as a slice (i.e., in-memory).
	for _, tc := range []struct {
		inMemory bool
	}{
		{true},
		{false},
	} {
		// 100 distinct privacy IDs contribute 0.0 and another 100 distinct privacy IDs
		// contribute 1.0 twice. MaxPartitionsContributed is 1, so only half of these
		// contributions will be kept and we expect equal number of 0.0's and 1.0s.
		triples := testutils.ConcatenateTriplesWithFloatValue(
			testutils.MakeTripleWithFloatValue(100, 0, 0.0),
			testutils.MakeTripleWithFloatValueStartingFromKey(100, 100, 0, 1.0),
			testutils.MakeTripleWithFloatValueStartingFromKey(100, 100, 0, 1.0))

		wantMetric := []testutils.PairIF64Slice{
			{0, []float64{0.0, 1.0}},
		}
		p, s, col, want := ptest.CreateList2(triples, wantMetric)

		epsilon := 900.0
		delta := 0.0
		lower := 0.0
		upper := 5.0
		ranks := []float64{0.49, 0.51}
		publicPartitionsSlice := []int{0}
		var publicPartitions any
		if tc.inMemory {
			publicPartitions = publicPartitionsSlice
		} else {
			publicPartitions = beam.CreateList(s, publicPartitionsSlice)
		}

		col = beam.ParDo(s, testutils.ExtractIDFromTripleWithFloatValue, col)

		// ε is split in two for noise and for partition selection, so we use 2*ε to get a Laplace noise with ε.
		pcol := MakePrivate(s, col, NewPrivacySpec(epsilon, delta))
		pcol = ParDo(s, testutils.TripleWithFloatValueToKV, pcol)
		quantilesParams := QuantilesParams{
			MaxPartitionsContributed:     1,
			MaxContributionsPerPartition: 1,
			MinValue:                     lower,
			MaxValue:                     upper,
			Ranks:                        ranks,
			PublicPartitions:             publicPartitions,
		}
		got := QuantilesPerKey(s, pcol, quantilesParams)

		want = beam.ParDo(s, testutils.PairIF64SliceToKV, want)
		if err := testutils.ApproxEqualsKVFloat64Slice(s, got, want, testutils.QuantilesTolerance(lower, upper)); err != nil {
			t.Fatalf("ApproxEqualsKVFloat64Slice in-memory=%t: got error %v", tc.inMemory, err)
		}
		if err := ptest.Run(p); err != nil {
			t.Errorf("QuantilesPerKey with partitions in-memory=%t did not bound cross-partition contributions correctly: %v", tc.inMemory, err)
		}
	}
}

// Checks that QuantilesPerKey clamps input values to MinValue and MaxValue.
func TestQuantilesPerKeyAppliesClamping(t *testing.T) {
	triples := testutils.ConcatenateTriplesWithFloatValue(
		testutils.MakeTripleWithFloatValue(100, 0, -5.0), // Will be clamped to 0.0
		testutils.MakeTripleWithFloatValue(100, 0, 10.0)) // Will be clamped to 5.0

	wantMetric := []testutils.PairIF64Slice{
		{0, []float64{0.0, 0.0, 5.0, 5.0}},
	}
	p, s, col, want := ptest.CreateList2(triples, wantMetric)
	col = beam.ParDo(s, testutils.ExtractIDFromTripleWithFloatValue, col)

	// ε=900, δ=10⁻²⁰⁰ and l0Sensitivity=1 gives a threshold of =2.
	epsilon := 900.0
	delta := 1e-200
	lower := 0.0
	upper := 5.0
	ranks := []float64{0.00, 0.25, 0.75, 1.00}

	// ε is split in two for noise and for partition selection, so we use 2*ε to get a Laplace noise with ε.
	pcol := MakePrivate(s, col, NewPrivacySpec(2*epsilon, delta))
	pcol = ParDo(s, testutils.TripleWithFloatValueToKV, pcol)
	got := QuantilesPerKey(s, pcol, QuantilesParams{
		MaxPartitionsContributed:     1,
		MaxContributionsPerPartition: 2,
		MinValue:                     lower,
		MaxValue:                     upper,
		Ranks:                        ranks,
	})

	want = beam.ParDo(s, testutils.PairIF64SliceToKV, want)
	if err := testutils.ApproxEqualsKVFloat64Slice(s, got, want, testutils.QuantilesTolerance(lower, upper)); err != nil {
		t.Fatalf("ApproxEqualsKVFloat64Slice: got error %v", err)
	}
	if err := ptest.Run(p); err != nil {
		t.Errorf("QuantilesPerKey did not clamp input values: %v", err)
	}
}

// Checks that QuantilesPerKey with partitions clamps input values to MinValue and MaxValue.
func TestQuantilesPerKeyWithPartitionsAppliesClamping(t *testing.T) {
	// We have two test cases, one for public partitions as a PCollection and one for public partitions as a slice (i.e., in-memory).
	for _, tc := range []struct {
		inMemory bool
	}{
		{true},
		{false},
	} {
		triples := testutils.ConcatenateTriplesWithFloatValue(
			testutils.MakeTripleWithFloatValue(100, 0, -5.0), // Will be clamped to 0.0
			testutils.MakeTripleWithFloatValue(100, 0, 10.0)) // Will be clamped to 5.0

		wantMetric := []testutils.PairIF64Slice{
			{0, []float64{0.0, 0.0, 5.0, 5.0}},
		}
		p, s, col, want := ptest.CreateList2(triples, wantMetric)

		epsilon := 900.0
		delta := 0.0
		lower := 0.0
		upper := 5.0
		ranks := []float64{0.00, 0.25, 0.75, 1.00}
		publicPartitionsSlice := []int{0}
		var publicPartitions any
		if tc.inMemory {
			publicPartitions = publicPartitionsSlice
		} else {
			publicPartitions = beam.CreateList(s, publicPartitionsSlice)
		}

		col = beam.ParDo(s, testutils.ExtractIDFromTripleWithFloatValue, col)

		// ε is split in two for noise and for partition selection, so we use 2*ε to get a Laplace noise with ε.
		pcol := MakePrivate(s, col, NewPrivacySpec(2*epsilon, delta))
		pcol = ParDo(s, testutils.TripleWithFloatValueToKV, pcol)
		quantilesParams := QuantilesParams{
			MaxPartitionsContributed:     1,
			MaxContributionsPerPartition: 2,
			MinValue:                     lower,
			MaxValue:                     upper,
			Ranks:                        ranks,
			PublicPartitions:             publicPartitions,
		}
		got := QuantilesPerKey(s, pcol, quantilesParams)

		want = beam.ParDo(s, testutils.PairIF64SliceToKV, want)
		if err := testutils.ApproxEqualsKVFloat64Slice(s, got, want, testutils.QuantilesTolerance(lower, upper)); err != nil {
			t.Fatalf("ApproxEqualsKVFloat64Slice in-memory=%t: got error %v", tc.inMemory, err)
		}
		if err := ptest.Run(p); err != nil {
			t.Errorf("QuantilesPerKey with partitions in-memory=%t did not clamp input values: %v", tc.inMemory, err)
		}
	}
}

func TestCheckQuantilesPerKeyParams(t *testing.T) {
	_, _, publicPartitions := ptest.CreateList([]int{0, 1})
	for _, tc := range []struct {
		desc                    string
		usesNewPrivacyBudgetAPI bool
		params                  QuantilesParams
		noiseKind               noise.Kind
		partitionType           reflect.Type
		wantErr                 bool
	}{
		{
			desc: "valid parameters",
			params: QuantilesParams{
				Epsilon:                      1.0,
				Delta:                        1e-5,
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: nil,
			wantErr:       false,
		},
		{
			desc: "negative epsilon",
			params: QuantilesParams{
				Epsilon:                      -1.0,
				Delta:                        1e-5,
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: nil,
			wantErr:       true,
		},
		{
			desc: "zero delta w/o public partitions",
			params: QuantilesParams{
				Epsilon:                      1.0,
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: nil,
			wantErr:       true,
		},
		{
			desc: "MaxValue < MinValue",
			params: QuantilesParams{
				Epsilon:                      1.0,
				Delta:                        1e-5,
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     6.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: nil,
			wantErr:       true,
		},
		{
			desc: "MaxValue = MinValue",
			params: QuantilesParams{
				Epsilon:                      1.0,
				Delta:                        1e-5,
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: nil,
			wantErr:       true,
		},
		{
			desc: "zero MaxContributionsPerPartition",
			params: QuantilesParams{
				Epsilon:                  1.0,
				Delta:                    1e-5,
				MaxPartitionsContributed: 1,
				MinValue:                 -5.0,
				MaxValue:                 5.0,
				Ranks:                    []float64{0.5},
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: nil,
			wantErr:       true,
		},
		{
			desc: "zero MaxPartitionsContributed",
			params: QuantilesParams{
				Epsilon:                      1.0,
				Delta:                        1e-5,
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: nil,
			wantErr:       true,
		},
		{
			desc: "No ranks",
			params: QuantilesParams{
				Epsilon:                      1.0,
				Delta:                        1e-5,
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: nil,
			wantErr:       true,
		},
		{
			desc: "Out of bound (<0.0 || >1.0) ranks",
			params: QuantilesParams{
				Epsilon:                      1.0,
				Delta:                        1e-5,
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.3, 1.5},
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: nil,
			wantErr:       true,
		},
		{
			desc: "non-zero delta w/ public partitions & Laplace",
			params: QuantilesParams{
				Epsilon:                      1.0,
				Delta:                        1e-5,
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
				PublicPartitions:             publicPartitions,
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: reflect.TypeOf(0),
			wantErr:       true,
		},
		{
			desc: "wrong partition type w/ public partitions as beam.PCollection",
			params: QuantilesParams{
				Epsilon:                      1.0,
				MaxContributionsPerPartition: 1,
				MaxPartitionsContributed:     1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
				PublicPartitions:             publicPartitions,
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: reflect.TypeOf(""),
			wantErr:       true,
		},
		{
			desc: "wrong partition type w/ public partitions as slice",
			params: QuantilesParams{
				Epsilon:                      1.0,
				MaxContributionsPerPartition: 1,
				MaxPartitionsContributed:     1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
				PublicPartitions:             []int{0},
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: reflect.TypeOf(""),
			wantErr:       true,
		},
		{
			desc: "wrong partition type w/ public partitions as array",
			params: QuantilesParams{
				Epsilon:                      1.0,
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
				PublicPartitions:             [1]int{0},
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: reflect.TypeOf(""),
			wantErr:       true,
		},
		{
			desc: "public partitions as something other than beam.PCollection, slice or array",
			params: QuantilesParams{
				Epsilon:                      1.0,
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
				PublicPartitions:             "",
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: reflect.TypeOf(""),
			wantErr:       true,
		},
		// Test cases for the new privacy budget API.
		{
			desc:                    "new API, valid parameters",
			usesNewPrivacyBudgetAPI: true,
			params: QuantilesParams{
				AggregationEpsilon:           1.0,
				PartitionSelectionParams:     PartitionSelectionParams{1.0, 1e-5},
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: nil,
			wantErr:       false,
		},
		{
			desc:                    "negative aggregationEpsilon",
			usesNewPrivacyBudgetAPI: true,
			params: QuantilesParams{
				AggregationEpsilon:           -1.0,
				PartitionSelectionParams:     PartitionSelectionParams{1.0, 1e-5},
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: nil,
			wantErr:       true,
		},
		{
			desc:                    "negative partitionSelectionEpsilon",
			usesNewPrivacyBudgetAPI: true,
			params: QuantilesParams{
				AggregationEpsilon:           1.0,
				PartitionSelectionParams:     PartitionSelectionParams{-1.0, 1e-5},
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: nil,
			wantErr:       true,
		},
		{
			desc:                    "new API, zero partitionSelectionDelta w/o public partitions",
			usesNewPrivacyBudgetAPI: true,
			params: QuantilesParams{
				AggregationEpsilon:           1.0,
				PartitionSelectionParams:     PartitionSelectionParams{1.0, 0},
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: nil,
			wantErr:       true,
		},
		{
			desc:                    "new API, MaxValue < MinValue",
			usesNewPrivacyBudgetAPI: true,
			params: QuantilesParams{
				AggregationEpsilon:           1.0,
				PartitionSelectionParams:     PartitionSelectionParams{1.0, 1e-5},
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     6.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: nil,
			wantErr:       true,
		},
		{
			desc:                    "new API, MaxValue = MinValue",
			usesNewPrivacyBudgetAPI: true,
			params: QuantilesParams{
				AggregationEpsilon:           1.0,
				PartitionSelectionParams:     PartitionSelectionParams{1.0, 1e-5},
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: nil,
			wantErr:       true,
		},
		{
			desc:                    "new API, zero MaxContributionsPerPartition",
			usesNewPrivacyBudgetAPI: true,
			params: QuantilesParams{
				AggregationEpsilon:       1.0,
				PartitionSelectionParams: PartitionSelectionParams{1.0, 1e-5},
				MaxPartitionsContributed: 1,
				MinValue:                 -5.0,
				MaxValue:                 5.0,
				Ranks:                    []float64{0.5},
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: nil,
			wantErr:       true,
		},
		{
			desc:                    "new API, zero MaxPartitionsContributed",
			usesNewPrivacyBudgetAPI: true,
			params: QuantilesParams{
				AggregationEpsilon:           1.0,
				PartitionSelectionParams:     PartitionSelectionParams{1.0, 1e-5},
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: nil,
			wantErr:       true,
		},
		{
			desc:                    "new API, no ranks",
			usesNewPrivacyBudgetAPI: true,
			params: QuantilesParams{
				AggregationEpsilon:           1.0,
				PartitionSelectionParams:     PartitionSelectionParams{1.0, 1e-5},
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: nil,
			wantErr:       true,
		},
		{
			desc:                    "new API, out of bound (<0.0 || >1.0) ranks",
			usesNewPrivacyBudgetAPI: true,
			params: QuantilesParams{
				AggregationEpsilon:           1.0,
				PartitionSelectionParams:     PartitionSelectionParams{1.0, 1e-5},
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.3, 1.5},
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: nil,
			wantErr:       true,
		},
		{
			desc:                    "new API, non-zero partitionSelectionDelta w/ public partitions",
			usesNewPrivacyBudgetAPI: true,
			params: QuantilesParams{
				AggregationEpsilon:           1.0,
				PartitionSelectionParams:     PartitionSelectionParams{0, 1e-5},
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
				PublicPartitions:             publicPartitions,
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: reflect.TypeOf(0),
			wantErr:       true,
		},
		{
			desc:                    "new API, non-zero partitionSelectionEpsilon w/ public partitions",
			usesNewPrivacyBudgetAPI: true,
			params: QuantilesParams{
				AggregationEpsilon:           1.0,
				PartitionSelectionParams:     PartitionSelectionParams{1.0, 0},
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
				PublicPartitions:             publicPartitions,
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: reflect.TypeOf(0),
			wantErr:       true,
		},
		{
			desc: "new API, wrong partition type w/ public partitions as beam.PCollection",
			params: QuantilesParams{
				AggregationEpsilon:           1.0,
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
				PublicPartitions:             publicPartitions,
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: reflect.TypeOf(""),
			wantErr:       true,
		},
		{
			desc: "new API, wrong partition type w/ public partitions as slice",
			params: QuantilesParams{
				AggregationEpsilon:           1.0,
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
				PublicPartitions:             []int{0},
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: reflect.TypeOf(""),
			wantErr:       true,
		},
		{
			desc: "new API, wrong partition type w/ public partitions as array",
			params: QuantilesParams{
				AggregationEpsilon:           1.0,
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
				PublicPartitions:             [1]int{0},
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: reflect.TypeOf(""),
			wantErr:       true,
		},
		{
			desc: "new API, public partitions as something other than beam.PCollection, slice or array",
			params: QuantilesParams{
				AggregationEpsilon:           1.0,
				MaxPartitionsContributed:     1,
				MaxContributionsPerPartition: 1,
				MinValue:                     -5.0,
				MaxValue:                     5.0,
				Ranks:                        []float64{0.5},
				PublicPartitions:             "",
			},
			noiseKind:     noise.LaplaceNoise,
			partitionType: reflect.TypeOf(""),
			wantErr:       true,
		},
	} {
		if err := checkQuantilesPerKeyParams(tc.params, tc.usesNewPrivacyBudgetAPI, tc.noiseKind, tc.partitionType); (err != nil) != tc.wantErr {
			t.Errorf("With %s, got=%v, wantErr=%t", tc.desc, err, tc.wantErr)
		}
	}
}

// The logic mirrors TestQuantilesPerKeyNoNoise, but with the new privacy budget API where
// clients specify aggregation budget and partition selection budget separately.
func TestQuantilesPerKeyNoNoiseTemp(t *testing.T) {
	// Arrange
	triples := testutils.ConcatenateTriplesWithFloatValue(
		testutils.MakeTripleWithFloatValue(100, 0, 1.0),
		testutils.MakeTripleWithFloatValue(100, 0, 4.0))

	wantMetric := []testutils.PairIF64Slice{
		{0, []float64{1.0, 1.0, 4.0, 4.0}},
	}
	p, s, col, want := ptest.CreateList2(triples, wantMetric)
	want = beam.ParDo(s, testutils.PairIF64SliceToKV, want)
	col = beam.ParDo(s, testutils.ExtractIDFromTripleWithFloatValue, col)

	// ε=900, δ=10⁻²⁰⁰ and l0Sensitivity=1 gives a threshold of =2.
	epsilon := 900.0
	delta := 1e-200
	lower := 0.0
	upper := 5.0
	ranks := []float64{0.00, 0.25, 0.75, 1.00}

	// Act
	spec, err := NewPrivacySpecTemp(PrivacySpecParams{
		AggregationEpsilon:        epsilon,
		AggregationDelta:          0,
		PartitionSelectionEpsilon: epsilon,
		PartitionSelectionDelta:   delta,
	})
	if err != nil {
		t.Fatalf("TestQuantilesPerKeyNoNoiseTemp: %v", err)
	}
	pcol := MakePrivate(s, col, spec)
	pcol = ParDo(s, testutils.TripleWithFloatValueToKV, pcol)
	got := QuantilesPerKey(s, pcol, QuantilesParams{
		MaxPartitionsContributed:     1,
		MaxContributionsPerPartition: 2,
		MinValue:                     lower,
		MaxValue:                     upper,
		Ranks:                        ranks,
	})

	// Assert
	if err := testutils.ApproxEqualsKVFloat64Slice(s, got, want, testutils.QuantilesTolerance(lower, upper)); err != nil {
		t.Fatalf("ApproxEqualsKVFloat64Slice: got error %v", err)
	}
	if err := ptest.Run(p); err != nil {
		t.Errorf("QuantilesPerKey did not return approximate quantile: %v", err)
	}
}

// The logic mirrors TestQuantilesPerKeyWithPartitionsNoNoise, but with the new privacy budget API where
// clients specify aggregation budget and partition selection budget separately.
func TestQuantilesPerKeyWithPartitionsNoNoiseTemp(t *testing.T) {
	// We have two test cases, one for public partitions as a PCollection and one for public partitions as a slice (i.e., in-memory).
	for _, tc := range []struct {
		inMemory bool
	}{
		{true},
		{false},
	} {
		// Arrange
		triples := testutils.ConcatenateTriplesWithFloatValue(
			testutils.MakeTripleWithFloatValue(100, 0, 1.0),
			testutils.MakeTripleWithFloatValue(100, 0, 4.0))

		wantMetric := []testutils.PairIF64Slice{
			{0, []float64{1.0, 1.0, 4.0, 4.0}},
		}
		p, s, col, want := ptest.CreateList2(triples, wantMetric)
		want = beam.ParDo(s, testutils.PairIF64SliceToKV, want)

		epsilon := 900.0
		lower := 0.0
		upper := 5.0
		ranks := []float64{0.00, 0.25, 0.75, 1.00}
		publicPartitionsSlice := []int{0}
		var publicPartitions any
		if tc.inMemory {
			publicPartitions = publicPartitionsSlice
		} else {
			publicPartitions = beam.CreateList(s, publicPartitionsSlice)
		}

		col = beam.ParDo(s, testutils.ExtractIDFromTripleWithFloatValue, col)

		// Act
		spec, err := NewPrivacySpecTemp(PrivacySpecParams{AggregationEpsilon: epsilon, AggregationDelta: 0})
		if err != nil {
			t.Fatalf("TestQuantilesPerKeyWithPartitionsNoNoiseTemp: %v", err)
		}
		pcol := MakePrivate(s, col, spec)
		pcol = ParDo(s, testutils.TripleWithFloatValueToKV, pcol)
		quantilesParams := QuantilesParams{
			MaxPartitionsContributed:     1,
			MaxContributionsPerPartition: 2,
			MinValue:                     lower,
			MaxValue:                     upper,
			Ranks:                        ranks,
			PublicPartitions:             publicPartitions,
		}
		got := QuantilesPerKey(s, pcol, quantilesParams)

		// Assert
		if err := testutils.ApproxEqualsKVFloat64Slice(s, got, want, testutils.QuantilesTolerance(lower, upper)); err != nil {
			t.Fatalf("ApproxEqualsKVFloat64Slice in-memory=%t: got error %v", tc.inMemory, err)
		}
		if err := ptest.Run(p); err != nil {
			t.Errorf("QuantilesPerKey with partitions in-memory=%t did not return approximate quantile: %v", tc.inMemory, err)
		}
	}
}

func TestQuantilesPerKeyPreThresholding(t *testing.T) {
	// Arrange
	// ε=10⁹, δ≈1 and l0Sensitivity=1 gives a threshold of ≈1.
	epsilon := 1e9
	delta := dpagg.LargestRepresentableDelta
	lower := 0.0
	upper := 5.0
	ranks := []float64{0.00, 0.25, 0.75, 1.00}
	spec, err := NewPrivacySpecTemp(PrivacySpecParams{
		AggregationEpsilon:        epsilon,
		AggregationDelta:          0,
		PartitionSelectionEpsilon: epsilon,
		PartitionSelectionDelta:   delta,
		PreThreshold:              10,
	})
	if err != nil {
		t.Fatalf("TestQuantilesPerKeyPreThresholding: %v", err)
	}
	triples := testutils.ConcatenateTriplesWithFloatValue(
		testutils.MakeTripleWithFloatValue(9, 0, 1.0),
		testutils.MakeTripleWithFloatValueStartingFromKey(10, 10, 1, 1.0),
		testutils.MakeTripleWithFloatValueStartingFromKey(10, 10, 1, 4.0))

	wantMetric := []testutils.PairIF64Slice{
		// The privacy ID count for partition 0 is 9, which is below the pre-threshold of 10: so it should be dropped.
		{1, []float64{1.0, 1.0, 4.0, 4.0}},
	}
	p, s, col, want := ptest.CreateList2(triples, wantMetric)
	want = beam.ParDo(s, testutils.PairIF64SliceToKV, want)
	col = beam.ParDo(s, testutils.ExtractIDFromTripleWithFloatValue, col)
	pcol := MakePrivate(s, col, spec)
	pcol = ParDo(s, testutils.TripleWithFloatValueToKV, pcol)

	// Act
	got := QuantilesPerKey(s, pcol, QuantilesParams{
		MaxPartitionsContributed:     1,
		MaxContributionsPerPartition: 2,
		MinValue:                     lower,
		MaxValue:                     upper,
		Ranks:                        ranks,
	})

	// Assert
	if err := testutils.ApproxEqualsKVFloat64Slice(s, got, want, testutils.QuantilesTolerance(lower, upper)); err != nil {
		t.Fatalf("ApproxEqualsKVFloat64Slice: got error %v", err)
	}
	if err := ptest.Run(p); err != nil {
		t.Errorf("TestQuantilesPerKeyPreThresholding: %v", err)
	}
}
