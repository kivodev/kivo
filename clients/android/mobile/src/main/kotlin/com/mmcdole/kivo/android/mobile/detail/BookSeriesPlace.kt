package com.mmcdole.kivo.android.mobile.detail

import com.mmcdole.kivo.api.models.BookInfo
import com.mmcdole.kivo.api.models.BookSeries
import com.mmcdole.kivo.api.models.BookSeriesRef
import com.mmcdole.kivo.api.models.WorkSummary

/** The series a book is mainly in: the one marked primary, else the first. */
internal fun BookInfo.primarySeries(): BookSeriesRef? = series.firstOrNull { it.primary } ?: series.firstOrNull()

/** The book's position in the series [seriesId] as written, such as "2" or "1.5". */
internal fun BookInfo.positionIn(seriesId: String): String? = series.firstOrNull { it.id == seriesId }?.position

/** Whether any of the series' books has a position, so each card keeps a line for one. */
internal fun BookSeries.hasPositions(): Boolean = items.any { it.book?.positionIn(id) != null }

/** The primary author more than half the books share, whose name the cards leave out; null when none does. */
internal fun BookSeries.commonAuthorId(): String? {
    val best = items.mapNotNull { it.book?.authors?.firstOrNull()?.id }.groupingBy { it }.eachCount().maxByOrNull { it.value } ?: return null
    return best.key.takeIf { best.value * 2 > items.size }
}

/** Whether any card names its author: some book's primary author isn't the common one. */
internal fun BookSeries.namesAuthors(): Boolean {
    val common = commonAuthorId() ?: return true
    return items.any { it.book?.authors?.firstOrNull()?.id != common }
}

/** Whether [book]'s card leaves its author line blank: it's by the series' common author. */
internal fun BookSeries.byCommonAuthor(book: WorkSummary): Boolean {
    val common = commonAuthorId() ?: return false
    return book.book?.authors?.firstOrNull()?.id == common
}
