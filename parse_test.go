package dicom_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"sort"
	"strings"
	"testing"

	"github.com/amitbet/dicom"
	"github.com/amitbet/dicom/dicomtag"
	"github.com/amitbet/dicom/dicomuid"
	"github.com/amitbet/dicom/element"
	"github.com/amitbet/dicom/write"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustReadFile(path string, options dicom.ParseOptions) *element.DataSet {
	p, err := dicom.NewParserFromFile(path, nil)
	p.Opts = options
	if err != nil {
		log.Panic(err)
	}

	data, err := p.Parse()
	// pixelData, _ := data.FindElementByName("PixelData")
	// _ = pixelData
	if err != nil {
		log.Panic(err)
	}
	return data
}

func TestAllFiles(t *testing.T) {
	dir, err := os.Open("examples")
	require.NoError(t, err)
	names, err := dir.Readdirnames(0)
	require.NoError(t, err)
	sort.Strings(names)
	for _, name := range names {
		if name == ".DS_Store" {
			continue
		}
		t.Logf("Reading %s", name)
		_ = mustReadFile("examples/"+name, dicom.ParseOptions{})
	}
}

func testWriteFile(t *testing.T, dcmPath, transferSyntaxUID string) {
	data := mustReadFile(dcmPath, dicom.ParseOptions{})
	dstPath := "/tmp/test.dcm"
	out, err := os.Create(dstPath)
	require.NoError(t, err)

	for i := range data.Elements {
		if data.Elements[i].Tag == dicomtag.TransferSyntaxUID {
			newElem := element.MustNewElement(dicomtag.TransferSyntaxUID, transferSyntaxUID)
			t.Logf("Setting transfer syntax UID from %v to %v",
				data.Elements[i].MustGetString(), newElem.MustGetString())
			data.Elements[i] = newElem
		}
	}
	err = write.DataSet(out, data)
	require.NoError(t, err)
	data2 := mustReadFile(dstPath, dicom.ParseOptions{})

	if len(data.Elements) != len(data2.Elements) {
		t.Errorf("Wrong # of elements: %v %v", len(data.Elements), len(data2.Elements))
		for _, elem := range data.Elements {
			if _, err := data2.FindElementByTag(elem.Tag); err != nil {
				t.Errorf("Tag %v found in org, but not in new", dicomtag.DebugString(elem.Tag))
			}
		}
		for _, elem := range data2.Elements {
			if _, err := data.FindElementByTag(elem.Tag); err != nil {
				t.Errorf("Tag %v found in new, but not in org", dicomtag.DebugString(elem.Tag))
			}
		}
	}
	for _, elem := range data.Elements {
		elem2, err := data2.FindElementByTag(elem.Tag)
		if err != nil {
			t.Error(err)
			continue
		}
		if elem.Tag == dicomtag.FileMetaInformationGroupLength {
			// This element is expected to change when the file is transcoded.
			continue
		}
		require.Equal(t, elem.String(), elem2.String())
	}
}

func TestWriteFile(t *testing.T) {
	// path := "examples/IM-0001-0001.dcm"
	//testWriteFile(t, "examples/CT-MONO2-16-ort.dcm", dicomuid.ExplicitVRBigEndian)
	//testWriteFile(t, "examples/CT-MONO2-16-ort.dcm", dicomuid.ImplicitVRLittleEndian)
	testWriteFile(t, "examples/CT-MONO2-16-ort.dcm", dicomuid.ExplicitVRLittleEndian)
}

func TestReadDataSet(t *testing.T) {
	data := mustReadFile("examples/IM-0001-0001.dcm", dicom.ParseOptions{})
	elem, err := data.FindElementByName("PatientName")
	require.NoError(t, err)
	assert.Equal(t, elem.MustGetString(), "TOUTATIX")
	elem, err = data.FindElementByName("TransferSyntaxUID")
	require.NoError(t, err)
	assert.Equal(t, elem.MustGetString(), "1.2.840.10008.1.2.4.91")
	assert.Equal(t, len(data.Elements), 98)
	elem, err = data.FindElementByTag(dicomtag.PixelData)

	assert.NoError(t, err)
}

func TestDatasetJson(t *testing.T) {
	data := mustReadFile("examples/IM-0001-0001.dcm", dicom.ParseOptions{})
	val, err := data.GetDataSetAsJsonObj(false, true, true)
	assert.NoError(t, err)

	b, err := json.Marshal(val)
	assert.NoError(t, err)

	fmt.Println(string(b))
}

// Test ReadOptions
func TestReadOptions(t *testing.T) {
	// Test Drop Pixel Data
	data := mustReadFile("examples/IM-0001-0001.dcm", dicom.ParseOptions{DropPixelData: true})
	_, err := data.FindElementByTag(dicomtag.PatientName)
	require.NoError(t, err)
	_, err = data.FindElementByTag(dicomtag.PixelData)
	require.Error(t, err)

	// Test Return Tags
	data = mustReadFile("examples/IM-0001-0001.dcm", dicom.ParseOptions{DropPixelData: true, ReturnTags: []dicomtag.Tag{dicomtag.StudyInstanceUID}})
	_, err = data.FindElementByTag(dicomtag.StudyInstanceUID)
	if err != nil {
		t.Error(err)
	}
	_, err = data.FindElementByTag(dicomtag.PatientName)
	if err == nil {
		t.Errorf("PatientName should not be present")
	}

	// Test Stop at Tag
	data = mustReadFile("examples/IM-0001-0001.dcm",
		dicom.ParseOptions{
			DropPixelData: true,
			// Study Instance UID Element tag is Tag{0x0020, 0x000D}
			StopAtTag: &dicomtag.StudyInstanceUID,
		})
	_, err = data.FindElementByTag(dicomtag.PatientName) // Patient Name Element tag is Tag{0x0010, 0x0010}
	if err != nil {
		t.Error(err)
	}
	_, err = data.FindElementByTag(dicomtag.SeriesInstanceUID) // Series Instance UID Element tag is Tag{0x0020, 0x000E}
	if err == nil {
		t.Errorf("SeriesInstanceUID should not be present")
	}
}

func BenchmarkParseSingle(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = mustReadFile("examples/IM-0001-0001.dcm", dicom.ParseOptions{})
	}
}

// parseDicomFile parses the dicom file and returns a Study object
func parseDicomFile(dicomFileName string) (*element.DataSet, error) {
	p, err := dicom.NewParserFromFile(dicomFileName, nil)
	if err != nil {
		return nil, err
	}

	dataset, err := p.Parse() // parse whole dicom
	if err != nil {
		return nil, err
	}
	return dataset, nil
}

func TestDicomToJson(t *testing.T) {
	//dir := "./dicoms/internet"
	dir := "./examples"
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Error("TestDicomToJson: Error in reading file: ", err)
	}

	for _, f := range files {
		fname := path.Join(dir, f.Name())
		if strings.HasPrefix(f.Name(), ".") {
			continue
		}

		ds, err := parseDicomFile(fname)
		//ds, err := ParseDicomFile("./dicoms/CHC/CT000006_anon")
		if err != nil {
			t.Error("Error in parsing dicom: ", fname, err)
		}

		json, err := ds.GetDataSetAsJson(true, true, false)
		if err != nil {
			t.Error("TestDicomToJson: Error in creating json: ", err)
		}
		_ = json
		fmt.Println("\"", fname, "\"", ":", json, ",")
	}
}

func TestDicomToJsonComparison(t *testing.T) {

	fname := "./examples/0124.000F_anon.img"
	ds, err := parseDicomFile(fname)
	if err != nil {
		t.Error("Error in parsing dicom: ", fname, err)
	}

	filter := element.GetDefaultMetadataTagFilter()
	json, err := ds.GetDataSetAsJsonFiltered(true, false, false, filter)
	if err != nil {
		t.Error("TestDicomToJson: Error in creating json: ", err)
	}

	fmt.Println("\"", fname, "\"", ":", json, ",")
}
