// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package store

import (
	"context"
	"testing"

	pb "github.com/datacommonsorg/mixer/proto"
	"github.com/datacommonsorg/mixer/util"
	"github.com/golang/protobuf/jsonpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestGetPopObs(t *testing.T) {
	data := map[string]string{}
	dcid := "geoId/06"
	key := util.BtPopObsPrefix + dcid
	btRow := `{
		"name": "Santa Clara",
		"populations": {
			"dc/p/zzlmxxtp1el87": {
				"popType": "Household",
				"numConstraints": 3,
				"propertyValues": {
					"householderAge": "Years45To64",
					"householderRace": "AsianAlone",
					"income": "USDollar35000To39999"
				},
				"observations": [
					{
						"marginOfError": 274,
						"measuredProp": "count",
						"measuredValue": 1352,
						"measurementMethod": "CensusACS5yrSurvey",
						"observationDate": "2017"
					},
					{
						"marginOfError": 226,
						"measuredProp": "count",
						"measuredValue": 1388,
						"measurementMethod": "CensusACS5yrSurvey",
						"observationDate": "2013"
					}
				]
			}
		},
		"observations": [
			{
				"meanValue": 4.1583,
				"measuredProp": "particulateMatter25",
				"measurementMethod": "CDCHealthTracking",
				"observationDate": "2014-04-04",
				"observedNode": "geoId/06085"
			},
			{
				"meanValue": 9.4461,
				"measuredProp": "particulateMatter25",
				"measurementMethod": "CDCHealthTracking",
				"observationDate": "2014-03-20",
				"observedNode": "geoId/06085"
			}
		]
	}`

	tableValue, err := util.ZipAndEncode(string(btRow))
	if err != nil {
		t.Errorf("util.ZipAndEncode(%+v) = %v", btRow, err)
	}
	data[key] = tableValue
	// Setup bigtable
	btClient, err := SetupBigtable(context.Background(), data)
	if err != nil {
		t.Errorf("SetupBigtable(...) = %v", err)
	}
	// Test
	s, err := &store{"", nil, nil, nil, nil, nil, nil, btClient.Open("dc"), NewCache()}, nil
	in := &pb.GetPopObsRequest{
		Dcid: dcid,
	}
	var out pb.GetPopObsResponse
	s.GetPopObs(context.Background(), in, &out)
	if diff := cmp.Diff(out.GetPayload(), tableValue); diff != "" {
		t.Errorf("GetPopObs() got diff: %v", diff)
	}
}

func TestGetPopObsCacheMerge(t *testing.T) {
	dcid := "geoId/06"
	key := util.BtPopObsPrefix + dcid

	// Setup bigtable
	baseData := map[string]string{}
	btRow := `{
		"name": "Santa Clara",
		"populations": {
			"dc/p/abcxyz": {
				"popType": "Household",
				"numConstraints": 3,
				"propertyValues": {
					"householderAge": "Years45To64",
					"householderRace": "AsianAlone",
					"income": "USDollar35000To39999"
				},
				"observations": [
					{
						"marginOfError": 226,
						"measuredProp": "count",
						"measuredValue": 1388,
						"measurementMethod": "CensusACS5yrSurvey",
						"observationDate": "2013"
					}
				]
			}
		}
	}`
	tableValue, err := util.ZipAndEncode(btRow)
	if err != nil {
		t.Errorf("util.ZipAndEncode(%+v) = %v", btRow, err)
	}
	baseData[key] = tableValue
	btClient, err := SetupBigtable(context.Background(), baseData)
	if err != nil {
		t.Errorf("SetupBigtable(...) = %v", err)
	}

	// branch cache data. Have observation on newer date.
	branchData := map[string]string{}
	branchCache := `{
		"name": "Santa Clara",
		"populations": {
			"dc/p/abcxyz": {
				"popType": "Household",
				"numConstraints": 3,
				"propertyValues": {
					"householderAge": "Years45To64",
					"householderRace": "AsianAlone",
					"income": "USDollar35000To39999"
				},
				"observations": [
					{
						"marginOfError": 274,
						"measuredProp": "count",
						"measuredValue": 1352,
						"measurementMethod": "CensusACS5yrSurvey",
						"observationDate": "2017"
					},
					{
						"marginOfError": 226,
						"measuredProp": "count",
						"measuredValue": 1388,
						"measurementMethod": "CensusACS5yrSurvey",
						"observationDate": "2013"
					}
				]
			}
		}
	}`
	branchCacheValue, err := util.ZipAndEncode(branchCache)
	if err != nil {
		t.Errorf("util.ZipAndEncode(%+v) = %v", branchCache, err)
	}
	branchData[key] = branchCacheValue
	// Test
	cache := Cache{}
	cache.Update(branchData)
	s, err := &store{
		"", nil, nil, nil, nil, nil, nil, btClient.Open("dc"), &cache}, nil

	var (
		in                       *pb.GetPopObsRequest
		out                      pb.GetPopObsResponse
		resultProto, expectProto pb.PopObsPlace
	)

	// Merge base cache and branch cache.
	in = &pb.GetPopObsRequest{
		Dcid: dcid,
	}
	s.GetPopObs(context.Background(), in, &out)

	if tmp, err := util.UnzipAndDecode(out.GetPayload()); err == nil {
		jsonpb.UnmarshalString(string(tmp), &resultProto)
	}
	jsonpb.UnmarshalString(branchCache, &expectProto)
	if diff := cmp.Diff(resultProto, expectProto, protocmp.Transform()); diff != "" {
		t.Errorf("GetPopObs() got diff %+v", diff)
	}

	// Only use the base cache.
	in = &pb.GetPopObsRequest{
		Dcid:   dcid,
		Option: &pb.Option{CacheChoice: pb.Option_BASE_CACHE_ONLY},
	}
	s.GetPopObs(context.Background(), in, &out)

	if tmp, err := util.UnzipAndDecode(out.GetPayload()); err == nil {
		jsonpb.UnmarshalString(string(tmp), &resultProto)
	}
	jsonpb.UnmarshalString(btRow, &expectProto)
	if diff := cmp.Diff(resultProto, expectProto, protocmp.Transform()); diff != "" {
		t.Errorf("GetPopObs() got diff %+v", diff)
	}
}

func TestGetPlaceObs(t *testing.T) {
	data := map[string]string{}
	key := util.BtPlaceObsPrefix + "City^2013^Person^gender^Male"
	btRow := `{
		"places":[
			{
				"name":"Stony Prairie CDP",
				"observations":[
					{
						"measuredProp":"count",
						"measuredValue":5000,
						"measurementMethod":"CensusACS5yrSurvey"
					}
				],
				"place":"geoId/3974832"
			},
			{
				"name":"Americus",
				"observations":[
					{
						"measuredProp":"count",
						"measuredValue":6000,
						"measurementMethod":"CensusACS5yrSurvey"
					}
				],
				"place":"geoId/2001675"
			}
		]
	}`

	tableValue, err := util.ZipAndEncode(string(btRow))
	if err != nil {
		t.Errorf("util.ZipAndEncode(%+v) = %v", btRow, err)
	}
	data[key] = tableValue
	// Setup bigtable
	btClient, err := SetupBigtable(context.Background(), data)
	if err != nil {
		t.Errorf("SetupBigtable(...) = %v", err)
	}

	var (
		in                       *pb.GetPlaceObsRequest
		out                      pb.GetPlaceObsResponse
		resultProto, expectProto pb.PopObsCollection
	)
	s, err := &store{"", nil, nil, nil, nil, nil, nil, btClient.Open("dc"), NewCache()}, nil

	// Base cache only.
	in = &pb.GetPlaceObsRequest{
		PlaceType:       "City",
		PopulationType:  "Person",
		ObservationDate: "2013",
		Pvs: []*pb.PropertyValue{
			&pb.PropertyValue{Property: "gender", Value: "Male"},
		},
	}
	s.GetPlaceObs(context.Background(), in, &out)
	if diff := cmp.Diff(out.GetPayload(), tableValue); diff != "" {
		t.Errorf("GetPlaceObs() got diff: %v", diff)
	}
	if tmp, err := util.UnzipAndDecode(out.GetPayload()); err == nil {
		jsonpb.UnmarshalString(string(tmp), &resultProto)
	}
	jsonpb.UnmarshalString(btRow, &expectProto)

	if diff := cmp.Diff(&resultProto, &expectProto, protocmp.Transform()); diff != "" {
		t.Errorf("GetPlaceObs() got diff %+v", diff)
	}
}

func TestGetPlaceObsCacheMerge(t *testing.T) {
	key := util.BtPlaceObsPrefix + "City^2013^Person^gender^Male"

	// No base data
	baseData := map[string]string{}
	btClient, err := SetupBigtable(context.Background(), baseData)
	if err != nil {
		t.Errorf("SetupBigtable(...) = %v", err)
	}

	// branch cache data. Have observation on newer date.
	branchData := map[string]string{}
	branchCache := `{
		"places":[
			{
				"name":"Stony Prairie CDP",
				"observations":[
					{
						"measuredProp":"count",
						"measuredValue":5000,
						"measurementMethod":"CensusACS5yrSurvey"
					}
				],
				"place":"geoId/3974832"
			},
			{
				"name":"Americus",
				"observations":[
					{
						"measuredProp":"count",
						"measuredValue":6000,
						"measurementMethod":"CensusACS5yrSurvey"
					}
				],
				"place":"geoId/2001675"
			}
		]
	}`
	branchCacheValue, err := util.ZipAndEncode(branchCache)
	if err != nil {
		t.Errorf("util.ZipAndEncode(%+v) = %v", branchCache, err)
	}
	branchData[key] = branchCacheValue
	// Test
	cache := Cache{}
	cache.Update(branchData)
	s, err := &store{
		"", nil, nil, nil, nil, nil, nil, btClient.Open("dc"), &cache}, nil
	in := &pb.GetPlaceObsRequest{
		PlaceType:       "City",
		PopulationType:  "Person",
		ObservationDate: "2013",
		Pvs: []*pb.PropertyValue{
			&pb.PropertyValue{Property: "gender", Value: "Male"},
		},
	}
	var out pb.GetPlaceObsResponse
	s.GetPlaceObs(context.Background(), in, &out)

	var resultProto, expectProto pb.PopObsPlace
	if tmp, err := util.UnzipAndDecode(out.GetPayload()); err == nil {
		jsonpb.UnmarshalString(string(tmp), &resultProto)
	}
	jsonpb.UnmarshalString(branchCache, &expectProto)

	if diff := cmp.Diff(resultProto, expectProto, protocmp.Transform()); diff != "" {
		t.Errorf("GetPlaceObs() got diff %+v", diff)
	}
}
