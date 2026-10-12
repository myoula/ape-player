package emby

import "time"

type UserData struct {
	LastPlayedDate        time.Time
	PlaybackPositionTicks int64
	IsFavorite            bool
	Played                bool
	PlayedPercentage      float64
}
type Stream struct {
	IsDefault      bool
	IsForced       bool
	Channels       int
	Index          int
	Type           string
	Codec          string
	Language       string
	DisplayTitle   string
	Title          string
	IsExternal     bool
	DeliveryUrl    string
	DeliveryMethod string
}
type MediaSource struct {
	DefaultAudioStreamIndex    *int
	DefaultSubtitleStreamIndex *int
	RequiredHttpHeaders        map[string]string
	AddApiKeyToDirectStreamUrl bool
	IsRemote                   bool
	ID                         string `json:"Id"`
	Name                       string
	Container                  string
	Path                       string
	Protocol                   string
	SupportsDirectPlay         bool
	SupportsDirectStream       bool
	SupportsTranscoding        bool
	DirectStreamUrl            string
	TranscodingUrl             string
	MediaStreams               []Stream
	RunTimeTicks               int64
}
type Item struct {
	CollectionType    string
	ID                string `json:"Id"`
	Name              string
	Type              string
	Overview          string
	ProductionYear    int
	PremiereDate      time.Time
	EndDate           time.Time
	Status            string
	CommunityRating   float64
	RunTimeTicks      int64
	SeriesName        string
	SeriesID          string `json:"SeriesId"`
	SeasonID          string `json:"SeasonId"`
	IndexNumber       int
	ParentIndexNumber int
	ImageTags         map[string]string
	PrimaryImageTag   string
	UserData          UserData
	MediaSources      []MediaSource
	MediaStreams      []Stream
}
type Items struct {
	Items            []Item
	TotalRecordCount int
}
type Auth struct {
	AccessToken string
	User        struct {
		ID   string `json:"Id"`
		Name string
	}
}
type PlaybackInfo struct {
	MediaSources  []MediaSource
	PlaySessionID string `json:"PlaySessionId"`
	ErrorCode     string
}
type Playback struct {
	Item          Item
	Source        MediaSource
	URL           string
	Headers       map[string]string
	SessionID     string
	Method        string
	StartTicks    int64
	AudioIndex    *int
	SubtitleIndex *int
}

const TicksPerSecond = 10000000

// YearFields requests the metadata used by movie and series year labels.
const YearFields = "ProductionYear,PremiereDate,EndDate,Status"
