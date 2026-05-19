; Adapted from ravsii/tree-sitter-d2 queries/highlights.scm
; (SHA 200434618a6bede20ebd4982aa4d4f1edeb0b5c1), with capture names
; remapped to Zed's recognized set.

; Comments
[
  (comment)
  (block_comment)
] @comment

; Strings (labels are user-facing label text in D2)
[
  (label)
  (label_codeblock)
  (label_array)
] @string

((label) @keyword
  (#any-of? @keyword
    "null"
    "Null"
    "NULL"
  )
)

((label) @string.special
  (#any-of? @string.special
    "suspend"
    "unsuspend"
    "top-left"
    "top-center"
    "top-right"
    "center-left"
    "center-right"
    "bottom-left"
    "bottom-center"
    "bottom-right"
    "outside-top-left"
    "outside-top-center"
    "outside-top-right"
    "outside-left-center"
    "outside-right-center"
    "outside-bottom-left"
    "outside-bottom-center"
    "outside-bottom-right"
  )
)

((label_array) @constant
  (#any-of? @constant
    "primary_key"
    "PK"
    "foreign_key"
    "FK"
    "unique"
    "UNQ"
    "NULL"
    "NOT NULL"
  )
)

(escape) @string.escape

; Identifiers are shape names / keys
(identifier) @function

((identifier) @function.builtin
  (#any-of? @function.builtin
    "3d"
    "animated"
    "bold"
    "border-radius"
    "class"
    "classes"
    "constraint"
    "d2-config"
    "d2-legend"
    "direction"
    "double-border"
    "fill"
    "fill-pattern"
    "filled"
    "font"
    "font-color"
    "font-size"
    "height"
    "italic"
    "label"
    "layers"
    "level"
    "link"
    "multiple"
    "near"
    "opacity"
    "scenarios"
    "shadow"
    "shape"
    "source-arrowhead"
    "steps"
    "stroke"
    "stroke-dash"
    "stroke-width"
    "style"
    "target-arrowhead"
    "text-transform"
    "tooltip"
    "underline"
    "vars"
    "width"
  )
)

((identifier) @keyword
  (#eq? @keyword "_")
)

[
 "$"
 "...$"
 "@"
 "...@"
] @keyword

[
 (glob_filter)
 (inverse_glob_filter)
 (visibility_mark)
] @keyword

; Import path token — closest Zed capture is @keyword
(import) @keyword

[(variable) (spread_variable)] @variable

(variable (identifier) @property)
(variable (identifier_chain (identifier) @property))

(spread_variable (identifier) @property)
(spread_variable (identifier_chain (identifier) @property))

[
  (glob)
  (recursive_glob)
  (global_glob)
] @string.special

(identifier
  (glob) @string.special.symbol)

(connection) @operator
(connection_identifier) @property
(integer) @number
(float) @number
(boolean) @boolean

(argument_name) @variable
(argument_type) @type

[
  "["
  "]"
  "("
  ")"
  "{"
  "}"
  "|"
  "||"
  "|||"
  "|`"
  "`|"
] @punctuation.bracket

[
  "."
  ","
  ":"
  ";"
] @punctuation.delimiter
