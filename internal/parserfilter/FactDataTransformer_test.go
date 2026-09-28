package parserfilter

import (
	"fmt"
	"testing"

	LagoonFact "lagoon.sh/insights-remote/internal"
)

func TestFactProcessor_TestMultipleFilters(t *testing.T) {

	fact := LagoonFact.Fact{
		EnvironmentId: "0",
		Name:          "testKey",
		Value:         "testvalue",
		Source:        "",
		Description:   "",
		KeyFact:       false,
		Type:          "",
		Category:      "",
	}

	fp1 := FactProcessor{
		Fact: fact,
		InsightsData: EnvironmentVariable{
			Key:   "testkey",
			Value: "testvalue",
		},
		hasErrorState: false,
		theError:      nil,
	}

	fp1.isOfType("NonExistentType").
		setFactField("Name", "friendlyname")

	if fp1.Fact.Name == "friendlyname" {
		t.Errorf("fp1 should not have been processed")
	}

	fp2 := FactProcessor{
		Fact: fact,
		InsightsData: EnvironmentVariable{
			Key:   "testkey",
			Value: "testvalue",
		},
		hasErrorState: false,
		theError:      nil,
	}

	fp2.isOfType("EnvironmentVariable").
		setFactField("Name", "friendlyname").setKeyFact(true)

	fact = fp2.Fact

	if fact.KeyFact != true {
		t.Errorf("fp2 should set fact to key")
	}
}

// ProcessLagoonFactAgainstRegisteredFilters
func TestFactProcessor_ProcessLagoonFactAgainstRegisteredFilters(t *testing.T) {

	fact := LagoonFact.Fact{
		EnvironmentId: "0",
		Name:          "testKey",
		Value:         "testvalue",
		Source:        "",
		Description:   "",
		KeyFact:       false,
		Type:          "",
		Category:      "",
	}

	poppedFactFilters := KeyFactFilters
	defer func() { KeyFactFilters = poppedFactFilters }()

	KeyFactFilters = []func(filter parserFilter) parserFilter{
		func(filter parserFilter) parserFilter {
			return filter.
				isOfType("EnvironmentVariable").setFactField("Name", "replaced")
		},
		func(filter parserFilter) parserFilter {
			return filter.
				isOfType("EnvironmentVariable").setKeyFact(true)
		},
	}

	returnFact, _ := ProcessLagoonFactAgainstRegisteredFilters(fact, EnvironmentVariable{
		Key:   "testkey",
		Value: "testvalue",
	})

	if returnFact.Name != "replaced" {
		t.Errorf("Resulting fact's name should have been replaced by 'replaced'")
	}

	if !returnFact.KeyFact {
		t.Errorf("Resulting fact should be a key fact")
	}

}

func TestFactProcessor_RecognisesType(t *testing.T) {

	fp := FactProcessor{
		Fact:          LagoonFact.Fact{},
		InsightsData:  ImageData{},
		hasErrorState: false,
		theError:      nil,
	}

	fp.isOfType("ImageData")

	if fp.hasError() {
		t.Errorf("Type should match 'ImageData'")
	}

}

func TestFactProcessor_FiltersByField(t *testing.T) {
	fp := FactProcessor{
		Fact:          LagoonFact.Fact{},
		InsightsData:  ImageData{Name: "Testing"},
		hasErrorState: false,
		theError:      nil,
	}

	fp.isOfType("ImageData").
		fieldContains("Name", "Testing")

	if fp.hasError() {
		t.Errorf("Field 'Name' should contain 'Testing'")
	}

}

func TestFactProcessor_SetKeyFact(t *testing.T) {

	fp := FactProcessor{
		Fact:          LagoonFact.Fact{},
		InsightsData:  ImageData{},
		hasErrorState: false,
		theError:      nil,
	}

	fp.setKeyFact(true)

	if fp.Fact.KeyFact != true {
		t.Errorf("Should have been set to key fact")
	}

}

func TestFactProcessor_SetFactValue(t *testing.T) {

	fp := FactProcessor{
		Fact:          LagoonFact.Fact{},
		InsightsData:  ImageData{},
		hasErrorState: false,
		theError:      nil,
	}

	fp.setFactField("Name", "namehere")

	if fp.Fact.Name != "namehere" {
		t.Errorf("Should be hable to set LagoonFact.Name")
	}

}

func TestFactProcessor_TestSetFriendlyName(t *testing.T) {

	fp1 := FactProcessor{
		Fact: LagoonFact.Fact{
			EnvironmentId: "0",
			Name:          "testKey",
			Value:         "testvalue",
			Source:        "",
			Description:   "",
			KeyFact:       false,
			Type:          "",
			Category:      "",
		},
		InsightsData: EnvironmentVariable{
			Key:   "testkey",
			Value: "testvalue",
		},
		hasErrorState: false,
		theError:      nil,
	}

	fp1.isOfType("EnvironmentVariable").
		fieldContains("Key", "testkey").
		setFactField("Name", "friendlyname")

	if fp1.Fact.Name != "friendlyname" {
		t.Errorf("Should be able to set LagoonFact.Name")
	}
}

func TestFactProcessor_TestExactMatchLookup(t *testing.T) {

	genfp := func() FactProcessor {
		return FactProcessor{
			Fact: LagoonFact.Fact{
				EnvironmentId: "0",
				Name:          "testKey",
				Value:         "testvalue",
				Source:        "",
				Description:   "",
				KeyFact:       false,
				Type:          "",
				Category:      "",
			},
			InsightsData: EnvironmentVariable{
				Key:   "testkey",
				Value: "testvalue",
			},
			hasErrorState: false,
			theError:      nil,
		}
	}

	fp1 := genfp()

	//First we test regex matching
	fp1.isOfType("EnvironmentVariable").
		fieldContains("Key", "test").
		setFactField("Name", "friendlyname")

	if fp1.Fact.Name != "friendlyname" {
		t.Errorf("The regex `test` should match `testkey`")
	}

	fp1 = genfp()
	//Now we test a non-exact match
	fp1.isOfType("EnvironmentVariable").
		fieldContainsExactMatch("Key", "test").
		setFactField("Name", "shouldnotbeset")

	if fp1.Fact.Name == "shouldnotbeset" {
		t.Errorf("The regex `test` should not match `shouldnotbeset`")
	}

	fp1 = genfp()
	//Now we test an exact match
	fp1.isOfType("EnvironmentVariable").
		fieldContainsExactMatch("Key", "testkey").
		setFactField("Name", "shouldbeset")

	fmt.Println(fp1.InsightsData)
	if fp1.Fact.Name != "shouldbeset" {
		t.Errorf("The regex `test` should match `shouldbeset`")
	}

}
