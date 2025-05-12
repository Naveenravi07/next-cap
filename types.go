package main

import "math"

type ValidationData struct {
	ImageID     string `json:"image_id"`     // Original image identifier
	ValidX      int    `json:"valid_x"`      // X coordinate where shape should be placed
	ValidY      int    `json:"valid_y"`      // Y coordinate where shape should be placed
	Tolerance   int    `json:"tolerance"`    // Allowed pixel deviation from exact position
	ShapeWidth  int    `json:"shape_width"`  // Width of the shape
	ShapeHeight int    `json:"shape_height"` // Height of the shape
}

func ValidateCaptchaAttempt(data ValidationData, attemptX, attemptY int) bool {
	xDiff := math.Abs(float64(data.ValidX - attemptX))
	yDiff := math.Abs(float64(data.ValidY - attemptY))

	return xDiff <= float64(data.Tolerance) && yDiff <= float64(data.Tolerance)
}
