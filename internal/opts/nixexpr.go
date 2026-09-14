package opts

// pathsExpr lists every option path under one subtree, and marks the ones a
// file defines. Bounded depth and guarded at each step: a single malformed
// option in nixpkgs must not take the scan down.
const pathsExpr = `a:
let
  isOpt = x: let r = builtins.tryEval ((x._type or "") == "option"); in r.success && r.value;
  walk = d: pfx: x:
    if d > 5 then [] else
    let ia = builtins.tryEval (x != null && builtins.isAttrs x); in
    if !ia.success || !ia.value then []
    else if isOpt x
      then let f = builtins.tryEval (x.files or []); in
           [ (pfx + (if f.success && builtins.isList f.value && f.value != [] then "\t" + builtins.concatStringsSep "," f.value else "")) ]
      else let ns = builtins.tryEval (builtins.attrNames x); in
           if !ns.success then [] else
           builtins.concatLists (map (n:
             let c = builtins.tryEval (walk (d + 1) (if pfx == "" then n else pfx + "." + n) (x.${n}));
             in if c.success then c.value else []) ns.value);
in walk 0 "" a`
