package pkgs

// unitsExpr maps each systemd unit to the store path its ExecStart launches.
const unitsExpr = `ss: builtins.listToAttrs (map (n:
  let r = builtins.tryEval (toString (ss.${n}.serviceConfig.ExecStart or ""));
  in { name = n; value = if r.success then r.value else ""; })
  (builtins.attrNames ss))`

// scanExpr collects every file that defines an option anywhere under one
// services.<name> subtree. Bounded depth, and every step is guarded: one
// malformed option in nixpkgs must not take the whole scan down.
const scanExpr = `a:
let
  isOpt = x: let r = builtins.tryEval ((x._type or "") == "option"); in r.success && r.value;
  scan = d: x:
    if d > 3 then [] else
    let ia = builtins.tryEval (x != null && builtins.isAttrs x); in
    if !ia.success || !ia.value then []
    else if isOpt x
      then let f = builtins.tryEval (x.files or []); in
           if f.success && builtins.isList f.value then f.value else []
      else let ns = builtins.tryEval (builtins.attrNames x); in
           if !ns.success then [] else
           builtins.concatLists (map (n:
             let c = builtins.tryEval (scan (d + 1) (x.${n}));
             in if c.success then c.value else []) ns.value);
in scan 0 a`

const sysPkgsExpr = `ps: map (p:
  let r = builtins.tryEval ((p.pname or p.name or "?") + "\t" + (p.version or ""));
  in if r.success then r.value else "") ps`

// homePkgsExpr flattens every home-manager user's home.packages into the same
// name-tab-version lines sysPkgsExpr emits. Each user is guarded: one broken
// package must not empty the whole report.
const homePkgsExpr = `us: builtins.concatLists (map (u:
  let r = builtins.tryEval (map (p:
    let q = builtins.tryEval ((p.pname or p.name or "?") + "\t" + (p.version or ""));
    in if q.success then q.value else "") us.${u}.home.packages);
  in if r.success then r.value else []) (builtins.attrNames us))`

// enableSweepExpr finds, in one evaluation, every services.<X> whose enable
// option some file defines. It reads option metadata rather than config, so
// the renamed-option machinery never aborts.
const enableSweepExpr = `opts:
let
  svc = opts.services;
  probe = n:
    let r = builtins.tryEval (
      let s = svc.${n};
          e = s.enable or null;
      in if e == null then null else { name = n; files = e.files or []; });
    in if r.success then r.value else null;
in builtins.listToAttrs (map (x: { name = x.name; value = x.files; })
     (builtins.filter (x: x != null && x.files != []) (map probe (builtins.attrNames svc))))`

// pkgOfExpr reads the package a service launches, for a service whose unit no
// name rule reaches: sshd comes from services.openssh, and opnix-secrets from
// services.onepassword-secrets.
const pkgOfExpr = `s:
let r = builtins.tryEval (
  let p = s.package or null;
  in if p == null then "" else (p.pname or p.name or "?") + "\t" + (p.version or ""));
in if r.success then r.value else ""`
