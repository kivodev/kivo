package query

import "github.com/mmcdole/kivo/internal/catalog"

// similar is the titles like a movie, series or book, for its More like
// this row: up to 20 titles of its own kind that the Viewer sees and
// hasn't played, best first. A title scores
//
//   - 100 less its place, from 0, in the work's TMDb recommendations,
//   - 50 for each director or author the two share,
//   - 15 for each narrator they share, or actor billed in the top 5 of both,
//   - 10 for each genre they share,
//   - 5 for each tag and each company they share,
//
// and one that scores under 20 is left out. Ties go by community rating,
// then by title. The other members of the work's collections and book
// series are left out, because the page has a row or a link for them. A
// series is played when every episode the Viewer sees is played, as its card
// says.
var similar = stmt("Similar", `
SELECT `+summaryColumns+`
FROM (
  SELECT m.work_id, sum(m.score) AS score
  FROM (
    SELECT x.work_id, 100 - r.ord AS score
    FROM work s
    CROSS JOIN work_recommendation r ON r.work_id = s.id -- CROSS JOIN reads the work's few rows first
    CROSS JOIN work_external_id x ON x.provider = 'tmdb' AND x.kind = s.kind AND x.ext_id = r.ext_id
    WHERE s.id = @work
    UNION ALL
    SELECT o.work_id, CASE WHEN c.role IN ('director', 'author') THEN 50 ELSE 15 END
    FROM credit c
    JOIN credit o ON o.person_id = c.person_id AND o.role = c.role AND (o.role <> 'actor' OR o.ord <= 5)
    WHERE c.work_id = @work AND (c.role IN ('director', 'author', 'narrator') OR c.role = 'actor' AND c.ord <= 5)
    UNION ALL
    SELECT o.work_id, 10 FROM work_genre g JOIN work_genre o ON o.genre_id = g.genre_id WHERE g.work_id = @work
    UNION ALL
    SELECT o.work_id, 5 FROM work_tag g JOIN work_tag o ON o.tag_id = g.tag_id WHERE g.work_id = @work
    UNION ALL
    SELECT o.work_id, 5 FROM work_company g JOIN work_company o ON o.company_id = g.company_id WHERE g.work_id = @work
  ) m
  WHERE m.work_id <> @work
  GROUP BY m.work_id
  HAVING sum(m.score) >= 20
) sc
JOIN work w ON w.id = sc.work_id
JOIN work_library wl ON wl.work_id = w.id
LEFT JOIN user_state us ON us.user_id = @user AND us.work_id = w.id
WHERE w.kind = (SELECT k.kind FROM work k WHERE k.id = @work AND k.kind IN ('movie', 'series', 'book'))
  AND NOT w.provisional
  AND `+visible+`
  AND CASE WHEN w.kind = 'series' THEN EXISTS (
      SELECT 1 FROM work e WHERE e.root_id = w.id AND e.kind = 'episode'
        AND EXISTS (SELECT 1 FROM work_library el WHERE el.work_id = e.id AND `+catalog.Granted("el.library_id")+`)
        AND NOT EXISTS (SELECT 1 FROM user_state eu WHERE eu.user_id = @user AND eu.work_id = e.id AND eu.played))
    ELSE NOT coalesce(us.played, 0) END
  AND NOT EXISTS (SELECT 1 FROM collection_work mc JOIN collection_work oc ON oc.collection_id = mc.collection_id
    WHERE mc.work_id = @work AND oc.work_id = w.id)
  AND NOT EXISTS (SELECT 1 FROM book_series_book sb JOIN book_series_book ob ON ob.series_id = sb.series_id
    WHERE sb.work_id = @work AND ob.work_id = w.id)
ORDER BY sc.score DESC, wl.rating_sort DESC, w.sort_title, w.id
LIMIT 20`)
