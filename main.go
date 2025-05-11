package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"math"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/ernyoke/imger/blur"
	"github.com/ernyoke/imger/edgedetection"
	"github.com/ernyoke/imger/grayscale"
	"github.com/ernyoke/imger/padding"
)

func drawLine(img *image.RGBA, p1, p2 image.Point, col color.Color) {
	dx := math.Abs(float64(p2.X - p1.X))
	dy := math.Abs(float64(p2.Y - p1.Y))

	x1, y1 := p1.X, p1.Y
	x2, y2 := p2.X, p2.Y

	sx := 1
	if x1 > x2 {
		sx = -1
	}

	sy := 1
	if y1 > y2 {
		sy = -1
	}

	err := dx - dy
	for {
		if image.Pt(x1, y1).In(img.Bounds()) {
			img.Set(x1, y1, col)
		}
		if x1 == x2 && y1 == y2 {
			break
		}

		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x1 += sx
		}
		if e2 < dx {
			err += dx
			y1 += sy
		}
	}
}

func superformula(phi float64, a, b float64, m float64, n1, n2, n3 float64) float64 {
	term1 := math.Pow(math.Abs(math.Cos(m*phi/4)/a), n2)
	term2 := math.Pow(math.Abs(math.Sin(m*phi/4)/b), n3)
	return math.Pow(term1+term2, -1/n1)
}

func shape_gen_in_region(x, y, w, h int) []image.Point {
	rand.Seed(time.Now().UnixNano())

	numPoints := 361
	points := make([]image.Point, 0, numPoints)

	a := rand.Float64()*0.5 + 0.6
	b := rand.Float64()*0.5 + 0.6
	m := float64(rand.Intn(10) + 3)
	n1 := rand.Float64()*3 + 1
	n2 := rand.Float64()*3 + 1
	n3 := rand.Float64()*3 + 1

	scale := float64(min(w, h)) * 0.4 // tighter fit
	cx := float64(x + w/2)
	cy := float64(y + h/2)

	for i := range numPoints {
		phi := float64(i) * math.Pi / 180
		r := superformula(phi, a, b, m, n1, n2, n3)

		xp := cx + scale*r*math.Cos(phi)
		yp := cy + scale*r*math.Sin(phi)

		points = append(points, image.Point{X: int(xp), Y: int(yp)})
	}

	return points
}

func SplitAny(s string, seps string) []string {
	splitter := func(r rune) bool {
		return strings.ContainsRune(seps, r)
	}
	return strings.FieldsFunc(s, splitter)
}

func findMaxHeat(heatmap [][]int) (maxVal, maxI, maxJ int) {
	maxVal = heatmap[0][0]
	maxI, maxJ = 0, 0

	for i := range len(heatmap) {
		for j := range len(heatmap[i]) {
			if heatmap[i][j] > maxVal {
				maxVal = heatmap[i][j]
				maxI, maxJ = i, j
			}
		}
	}

	return
}

func main() {
	filePath := os.Args[1]
	outBasePath := "assets/prod/"

	fmt.Println("[INFO] Reading file from ", filePath)

	file, err := os.Open(filePath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	fileName := SplitAny(file.Name(), "/ .")[2]

	img, _, err := image.Decode(file)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	//apply gaussian filter here
	grey := grayscale.Grayscale(img)
	blur_img, err := blur.GaussianBlurGray(grey, 7, 3,padding.BorderConstant)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	edge_img, err := edgedetection.CannyGray(blur_img, 10, 100, 3)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}

	// Calculate Heatmap
	blockRows := 5
	blockCols := 5

	blockW := w / blockCols
	blockH := h / blockRows

	fmt.Println("Blockw = ", blockW, "  BlockH = ", blockH)

	heatmap := make([][]int, blockRows)
	for i := range heatmap {
		heatmap[i] = make([]int, blockCols)
	}

	for y := range h {
		for x := range w {
			gray := edge_img.GrayAt(x, y).Y
			if gray > 128 {
				row := y / blockH
				col := x / blockW

				if row < blockRows && col < blockCols {
					heatmap[row][col]++
				}
			}
		}
	}

	fmt.Println("[INFO] Heatmap calculated ")
	for i := range blockRows {
		fmt.Println("")
		for j := range blockCols {
			fmt.Print(" ", heatmap[i][j], " ")
		}
	}
	fmt.Println()

	max, row, col := findMaxHeat(heatmap)
	x := col * blockW
	y := row * blockH

	fmt.Println("Max heat on x=", x, " y =", y, " heat=", max)

	// Generate random shape

	shapePoints := shape_gen_in_region(x, y, blockW, blockH)
	fmt.Println("[INFO] Generated shape with", len(shapePoints), "points.")

	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, img, bounds.Min, draw.Src)
	red := color.RGBA{255, 0, 0, 255}

	for i := range len(shapePoints) - 1 {
		drawLine(dst, shapePoints[i], shapePoints[i+1], red)
	}
	drawLine(dst, shapePoints[0], shapePoints[len(shapePoints)-1], red)

	// Save  images

	edgeFileName := outBasePath + fileName + "_edge.png"
	edgeFileOut, err := os.Create(edgeFileName)

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	png.Encode(edgeFileOut, edge_img)
	fmt.Println("[INFO] Edge file saved at ", edgeFileName)
	defer edgeFileOut.Close()

	outFileName := outBasePath + fileName + "_out.png"
	outFile, err := os.Create(outFileName)

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	png.Encode(outFile, dst)
	defer outFile.Close()
	defer file.Close()

	fmt.Println("[INFO] Captcha image saved at ", outFileName)
}
