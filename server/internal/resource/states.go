package resource

type PublicationState string

const (
	Draft      PublicationState = "draft"
	Pending    PublicationState = "pending"
	Published  PublicationState = "published"
	Restricted PublicationState = "restricted"
	Removed    PublicationState = "removed"
)

func (s PublicationState) Valid() bool {
	switch s {
	case Draft, Pending, Published, Restricted, Removed:
		return true
	}
	return false
}

type Lifecycle string

const (
	Active       Lifecycle = "active"
	Inactive     Lifecycle = "inactive"
	Discontinued Lifecycle = "discontinued"
	Delisted     Lifecycle = "delisted"
	Archived     Lifecycle = "archived"
	Unknown      Lifecycle = "unknown"
)

func (s Lifecycle) Valid() bool {
	switch s {
	case Active, Inactive, Discontinued, Delisted, Archived, Unknown:
		return true
	}
	return false
}

type ContentRating string

const (
	General  ContentRating = "general"
	Mature   ContentRating = "mature"
	Explicit ContentRating = "explicit"
)

func (s ContentRating) Valid() bool {
	switch s {
	case General, Mature, Explicit:
		return true
	}
	return false
}

type SourceType string

const (
	SourceOfficial  SourceType = "official"
	SourceStore     SourceType = "store"
	SourceArchive   SourceType = "archive"
	SourceMirror    SourceType = "mirror"
	SourceCommunity SourceType = "community"
	SourceExternal  SourceType = "external"
	SourceUnknown   SourceType = "unknown"
)

func (s SourceType) Valid() bool {
	switch s {
	case SourceOfficial, SourceStore, SourceArchive, SourceMirror, SourceCommunity, SourceExternal, SourceUnknown:
		return true
	}
	return false
}

type SourceAvailabilityState string

const (
	SourceActive      SourceAvailabilityState = "active"
	SourceUnavailable SourceAvailabilityState = "unavailable"
	SourceBroken      SourceAvailabilityState = "broken"
	SourceRemoved     SourceAvailabilityState = "removed"
	SourceRestricted  SourceAvailabilityState = "restricted"
)

func (s SourceAvailabilityState) Valid() bool {
	switch s {
	case SourceActive, SourceUnavailable, SourceBroken, SourceRemoved, SourceRestricted:
		return true
	}
	return false
}

type SourceRightsStatus string

const (
	RightsUnknown    SourceRightsStatus = "unknown"
	CreatorProvided  SourceRightsStatus = "creator_provided"
	RightsConfirmed  SourceRightsStatus = "confirmed"
	RightsDisputed   SourceRightsStatus = "disputed"
	RightsReview     SourceRightsStatus = "rights_review"
	RemovedByRequest SourceRightsStatus = "removed_by_request"
)

func (s SourceRightsStatus) Valid() bool {
	switch s {
	case RightsUnknown, CreatorProvided, RightsConfirmed, RightsDisputed, RightsReview, RemovedByRequest:
		return true
	}
	return false
}

type RelationType string

const (
	PartOf      RelationType = "part_of"
	SuccessorOf RelationType = "successor_of"
	DerivedFrom RelationType = "derived_from"
	RelatedTo   RelationType = "related_to"
)

func (s RelationType) Valid() bool {
	switch s {
	case PartOf, SuccessorOf, DerivedFrom, RelatedTo:
		return true
	}
	return false
}
