package report

import (
	"fmt"
	"math"
	"strings"
)

// PieSlice represents a single slice of a pie chart.
type PieSlice struct {
	Label string
	Value float64
	Color string
}

// palette for SVG pie chart slices.
var palette = []string{
	"#4e79a7", "#f28e2b", "#e15759", "#76b7b2",
	"#59a14f", "#edc948", "#b07aa1", "#ff9da7",
	"#9c755f", "#bab0ac",
}

// svgPieChart generates an inline SVG pie chart from the given slices.
// width/height are in pixels. Returns raw SVG markup.
func svgPieChart(title string, slices []PieSlice, width, height int) string {
	if len(slices) == 0 {
		return ""
	}

	var total float64
	for _, s := range slices {
		total += s.Value
	}
	if total == 0 {
		return ""
	}

	cx := float64(width) / 2
	cy := float64(height)/2 - 10 // leave room for title
	r := math.Min(cx, cy) - 30   // padding for legend

	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height)
	fmt.Fprintf(&sb, `<text x="%g" y="18" text-anchor="middle" font-size="14" font-weight="bold" fill="#333">%s</text>`, cx, title)

	startAngle := -math.Pi / 2
	for i, s := range slices {
		frac := s.Value / total
		angle := frac * 2 * math.Pi
		endAngle := startAngle + angle

		x1 := cx + r*math.Cos(startAngle)
		y1 := cy + r*math.Sin(startAngle) + 25
		x2 := cx + r*math.Cos(endAngle)
		y2 := cy + r*math.Sin(endAngle) + 25

		largeArc := 0
		if angle > math.Pi {
			largeArc = 1
		}

		color := palette[i%len(palette)]

		if len(slices) == 1 {
			// Full circle — draw a circle instead of a path.
			fmt.Fprintf(&sb, `<circle cx="%g" cy="%g" r="%g" fill="%s"/>`,
				cx, cy+25, r, color)
		} else {
			fmt.Fprintf(&sb, `<path d="M %g %g A %g %g 0 %d 1 %g %g L %g %g Z" fill="%s"/>`,
				x1, y1, r, r, largeArc, x2, y2, cx, cy+25, color)
		}

		// Legend item.
		ly := float64(height) - float64(len(slices)-i)*16
		fmt.Fprintf(&sb, `<rect x="5" y="%g" width="10" height="10" fill="%s"/>`, ly, color)
		fmt.Fprintf(&sb, `<text x="20" y="%g" font-size="11" fill="#333">%s: %.0f (%.0f%%)</text>`,
			ly+10, s.Label, s.Value, frac*100)

		startAngle = endAngle
	}

	sb.WriteString(`</svg>`)
	return sb.String()
}

// countDistribution groups values and returns slices for a pie chart.
func countDistribution(values []int, labels map[int]string) []PieSlice {
	counts := make(map[int]int)
	for _, v := range values {
		counts[v]++
	}
	var slices []PieSlice
	for val := 1; val <= 4; val++ {
		if c, ok := counts[val]; ok && c > 0 {
			label := fmt.Sprintf("%d", val)
			if l, ok := labels[val]; ok {
				label = l
			}
			slices = append(slices, PieSlice{
				Label: label,
				Value: float64(c),
			})
		}
	}
	return slices
}

// BuildProbabilityChart creates an SVG pie chart for risk probability distribution.
func BuildProbabilityChart(probabilities []int) string {
	labels := map[int]string{1: "Низкая (1)", 2: "Средняя (2)", 3: "Высокая (3)", 4: "Критическая (4)"}
	slices := countDistribution(probabilities, labels)
	return svgPieChart("Вероятности рисков", slices, 300, 250)
}

// BuildImpactChart creates an SVG pie chart for risk impact distribution.
func BuildImpactChart(impacts []int) string {
	labels := map[int]string{1: "Минимальное (1)", 2: "Умеренное (2)", 3: "Значительное (3)", 4: "Критическое (4)"}
	slices := countDistribution(impacts, labels)
	return svgPieChart("Влияние рисков", slices, 300, 250)
}

// BuildCoefficientChart creates an SVG pie chart for risk coefficient distribution.
func BuildCoefficientChart(coefficients []float64) string {
	// Group coefficients into buckets.
	buckets := map[string]int{}
	order := []string{"1.05", "1.10", "1.20", "1.30"}
	for _, c := range coefficients {
		switch {
		case c >= 1.30:
			buckets["1.30"]++
		case c >= 1.20:
			buckets["1.20"]++
		case c >= 1.10:
			buckets["1.10"]++
		default:
			buckets["1.05"]++
		}
	}
	var slices []PieSlice
	labels := map[string]string{
		"1.05": "×1.05 (низкий)",
		"1.10": "×1.10 (средний)",
		"1.20": "×1.20 (высокий)",
		"1.30": "×1.30 (критический)",
	}
	for _, key := range order {
		if c := buckets[key]; c > 0 {
			slices = append(slices, PieSlice{
				Label: labels[key],
				Value: float64(c),
			})
		}
	}
	return svgPieChart("Коэффициенты рисков", slices, 300, 250)
}
