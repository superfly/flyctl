package plan

type ObjectStoragePlan struct {
	TigrisObjectStorage *TigrisObjectStoragePlan `json:"tigris_object_storage"`
}

func (p *ObjectStoragePlan) Provider() any {
	if p == nil {
		return nil
	}
	if p.TigrisObjectStorage != nil {
		return p.TigrisObjectStorage
	}
	return nil
}

func DefaultObjectStorage(plan *LaunchPlan) ObjectStoragePlan {
	return ObjectStoragePlan{
		TigrisObjectStorage: &TigrisObjectStoragePlan{},
	}
}

type TigrisObjectStoragePlan struct {
	Name              string `json:"name"`
	Public            bool   `json:"public"`
	Accelerate        bool   `json:"accelerate"`
	WebsiteDomainName string `json:"website_domain_name"`
}
