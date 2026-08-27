package adminsvc

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/yixian-huang/imgli/internal/imaging"
	"github.com/yixian-huang/imgli/internal/model"
	"github.com/yixian-huang/imgli/internal/service/settings"
	"github.com/yixian-huang/imgli/internal/service/upload"
)

func TestProcessingPutGetSettings(t *testing.T) {
	db := model.TestDB(t)
	svc := New(db)

	cfg := upload.Processing{
		TextWatermark: upload.TextWatermark{
			Enabled:   true,
			Text:      "白栗©",
			Position:  "bc",
			Opacity:   0.5,
			SizeRatio: 0.08,
		},
		MaxEdge: 2048,
	}
	if err := svc.PutSettings(map[string]json.RawMessage{
		model.SettingProcessing: rawJSON(t, cfg),
	}); err != nil {
		t.Fatal(err)
	}

	var got upload.Processing
	if err := settings.New(db).Get(model.SettingProcessing, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, cfg) {
		t.Errorf("落库 = %+v, want %+v", got, cfg)
	}

	m, err := svc.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	proc, ok := m["processing"].(upload.Processing)
	if !ok {
		t.Fatalf("processing 类型 = %T", m["processing"])
	}
	if !reflect.DeepEqual(proc, cfg) {
		t.Errorf("GetSettings processing = %+v, want %+v", proc, cfg)
	}
}

func TestProcessingPutSettingsInvalid(t *testing.T) {
	svc := New(model.TestDB(t))
	cfg := upload.DefaultProcessing()
	cfg.TextWatermark.Opacity = 0
	err := svc.PutSettings(map[string]json.RawMessage{
		model.SettingProcessing: rawJSON(t, cfg),
	})
	if !errors.Is(err, upload.ErrProcessingInvalid) {
		t.Errorf("err=%v want ErrProcessingInvalid", err)
	}
}

func TestProcessingGetSettingsDefault(t *testing.T) {
	svc := New(model.TestDB(t))
	m, err := svc.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	proc, ok := m["processing"].(upload.Processing)
	if !ok {
		t.Fatalf("processing 类型 = %T", m["processing"])
	}
	if !reflect.DeepEqual(proc, upload.DefaultProcessing()) {
		t.Errorf("processing = %+v want DefaultProcessing()", proc)
	}
}

func TestGetSettingsProcessingCapabilitiesHeicDecode(t *testing.T) {
	svc := New(model.TestDB(t))
	m, err := svc.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	caps, ok := m["processing_capabilities"].(map[string]any)
	if !ok {
		t.Fatalf("processing_capabilities type = %T", m["processing_capabilities"])
	}
	got, ok := caps["heic_decode"]
	if !ok {
		t.Fatal("missing heic_decode")
	}
	if got != imaging.HeicDecodeAvailable() {
		t.Errorf("heic_decode = %v, want %v", got, imaging.HeicDecodeAvailable())
	}
}
