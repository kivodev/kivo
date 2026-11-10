# Naming fixtures

Permissively licensed filename-parser test cases, vendored unmodified for
`internal/parse`. Each set keeps its upstream license next to it.

| Path | Source | License |
| --- | --- | --- |
| `guessit/{ptt,thcolin,go-ptn,ptn}.json` | guessit's cross-parser sets, `guessit/test/cross_parser/` at guessit-io/guessit@3c73072b. Each file re-expresses another parser's tests in guessit's vocabulary; `guessit/NOTICE.md` names the upstream projects. | MIT, see `guessit/LICENSE.*` |
| `anitomy/data.json` | erengy/anitomy `test/data.json` at 8498b538 | MPL-2.0, see `anitomy/LICENSE` |

guessit's own YAML tests and its `baseline.json` are LGPL and aren't vendored;
guessit's `anitomy.json` is left out in favour of anitomy's original file.
