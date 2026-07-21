package service

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func TestTaskTokenQuotaUsesSubmittedRatioSnapshot(t *testing.T) {
	task := &model.Task{
		Group: "group-that-may-change-later",
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				OriginModelName:   "ratio-that-may-change-later",
				ModelRatio:        2,
				GroupRatio:        3,
				ChannelRatio:      2,
				ChannelModelRatio: 0.5,
				OtherRatios:       map[string]float64{"duration": 4},
			},
		},
	}

	quota, _, ok := taskTokenQuotaFromSnapshot(task, 10)
	if !ok {
		t.Fatal("submitted ratio snapshot was not recognized")
	}
	if quota != 240 {
		t.Fatalf("snapshot quota = %d, want 10*2*3*2*0.5*4 = 240", quota)
	}
}

func TestTaskTokenQuotaPreservesFreeGroupSnapshot(t *testing.T) {
	oldRatios, err := json.Marshal(ratio_setting.GetModelRatioCopy())
	if err != nil {
		t.Fatal(err)
	}
	if err := ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4":15}`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ratio_setting.UpdateModelRatioByJSONString(string(oldRatios)) })

	task := &model.Task{
		Group: "default",
		Properties: model.Properties{
			OriginModelName: "gpt-4",
		},
		PrivateData: model.TaskPrivateData{
			BillingContext: &model.TaskBillingContext{
				ModelRatio:        2,
				GroupRatio:        0,
				ChannelRatio:      1,
				ChannelModelRatio: 1,
			},
		},
	}

	quota, _, ok := taskTokenQuota(task, 100)
	if !ok {
		t.Fatal("free submitted ratio snapshot was not recognized")
	}
	if quota != 0 {
		t.Fatalf("free group snapshot quota = %d, want 0", quota)
	}
}
