package emby

import (
	"context"
	"net/url"
	"sort"
	"strconv"
)

// HideFromResume hides an item from Continue Watching without deleting media.
func (c *Client) HideFromResume(ctx context.Context, id string, hide bool) error {
	return c.Do(ctx, "POST", "Users/"+c.UserID+"/Items/"+id+"/HideFromResume", url.Values{"Hide": {strconv.FormatBool(hide)}}, nil, nil)
}

// RecentlyPlayed includes unfinished and completed videos. IsPlayed would omit
// unfinished videos; a last-played timestamp distinguishes history from unseen items.
func (c *Client) RecentlyPlayed(ctx context.Context, limit int) ([]Item, error) {
	if limit <= 0 {
		limit = 18
	}
	result, e := c.List(ctx, "Users/"+c.UserID+"/Items", url.Values{
		"Recursive": {"true"}, "IncludeItemTypes": {"Movie,Episode"}, "SortBy": {"DatePlayed"}, "SortOrder": {"Descending"}, "Limit": {strconv.Itoa(limit)}, "EnableUserData": {"true"}, "Fields": {"Overview,UserDataLastPlayedDate," + YearFields},
	})
	if e != nil {
		return nil, e
	}
	items := make([]Item, 0, len(result.Items))
	for _, item := range result.Items {
		if !item.UserData.LastPlayedDate.IsZero() {
			items = append(items, item)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].UserData.LastPlayedDate.After(items[j].UserData.LastPlayedDate) })
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}
