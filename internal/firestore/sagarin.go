package firestore

import (
	"time"

	"cloud.google.com/go/firestore"
)

// SagarinModelParameters represents Sagarin home field advantages for one scraping of Sagarin rankings.
type SagarinModelParameters struct {
	// Timestamp is the time the scraped model parameters were saved to Firestore.
	Timestamp time.Time `firestore:"timestamp,serverTimestamp"`

	// TimeDownload is the time the model parameters were downloaded
	TimeDownloaded time.Time `firestore:"time_downloaded,omitempty"`

	// URL is the URL that was scraped
	URL string `firestore:"url_scraped,omitempty"`

	// RatingHomeAdvantage is the number of points the home team for a given contest should be given if using the default "ratings" method for prediction.
	RatingHomeAdvantage float64 `firestore:"home_advantage_rating"`

	// PointsHomeAdvantage is the number of points the home team for a given contest should be given if using the "points-only" method for prediction.
	PointsHomeAdvantage float64 `firestore:"home_advantage_points"`

	// GoldenMeanHomeAdvantage is the number of points the home team for a given contest should be given if using the "golden mean" method for prediction.
	GoldenMeanHomeAdvantage float64 `firestore:"home_advantage_golden_mean"`

	// RecentHomeAdvantage is the number of points the home team for a given contest should be given if using the "recent" method for prediction.
	RecentHomeAdvantage float64 `firestore:"home_advantage_recent"`
}

// SagarinRating represents a team's Sagarin ratings in Firestore.
type SagarinRating struct {
	// Team is a reference to the Firestore document of the team to which these ratings apply.
	Team *firestore.DocumentRef `firestore:"team"`

	// Rating is the number of points this team is predicted to score against an average NCAA Division I-A team at a neutral site using the default "ratings" method for prediction.
	Rating float64 `firestore:"rating"`

	// Points is the number of points this team is predicted to score against an average NCAA Division I-A team at a neutral site using the "points-only" method for prediction.
	Points float64 `firestore:"points"`

	// GoldenMean is the number of points this team is predicted to score against an average NCAA Division I-A team at a neutral site using the "golden mean" method for prediction.
	GoldenMean float64 `firestore:"golden_mean"`

	// Recent is the number of points this team is predicted to score against an average NCAA Division I-A team at a neutral site using the "recent" method for prediction.
	Recent float64 `firestore:"recent"`
}
