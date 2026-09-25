#!/usr/bin/env python3
"""Minimal NvS subset interpreter in Python — polyglot bootstrap path."""
import re, sys

class Tok:
    def __init__(self, t, v): self.t, self.v = t, v

def tokenize(src):
    i, n, out = 0, len(src), []
    while i < n:
        c = src[i]
        if c in " \t\n\r":
            i += 1
        elif c.isdigit():
            j = i
            while j < n and src[j].isdigit(): j += 1
            out.append(Tok("INT", src[i:j])); i = j
        elif c.isalpha() or c == "_":
            j = i
            while j < n and (src[j].isalnum() or src[j] == "_"): j += 1
            w = src[i:j]
            tt = "KW" if w in ("let","print","if","else","while","fn","return","true","false","null") else "IDENT"
            out.append(Tok(tt, w)); i = j
        elif c in "\"'":
            j = i + 1
            while j < n and src[j] != c: j += 1
            out.append(Tok("STR", src[i+1:j])); i = j + 1
        else:
            two = src[i:i+2]
            if two in ("==","!=","<=",">=","&&","||"):
                out.append(Tok("OP", two)); i += 2
            else:
                out.append(Tok("OP", c)); i += 1
    out.append(Tok("EOF", ""))
    return out

class Mini:
    def __init__(self, src):
        self.toks = tokenize(src)
        self.pos = 0
        self.env = {}
        self.funcs = {}
    def peek(self): return self.toks[self.pos]
    def adv(self): t = self.toks[self.pos]; self.pos += 1; return t
    def mop(self, v):
        if self.peek().t == "OP" and self.peek().v == v: self.adv(); return True
        return False
    def mkw(self, v):
        if self.peek().t == "KW" and self.peek().v == v: self.adv(); return True
        return False
    def primary(self):
        t = self.peek()
        if t.t == "INT": self.adv(); return int(t.v)
        if t.t == "STR": self.adv(); return t.v
        if t.t == "KW" and t.v == "true": self.adv(); return True
        if t.t == "KW" and t.v == "false": self.adv(); return False
        if t.t == "KW" and t.v == "null": self.adv(); return None
        if self.mop("["):
            arr = []
            if not (self.peek().t == "OP" and self.peek().v == "]"):
                arr.append(self.expr())
                while self.mop(","): arr.append(self.expr())
            self.mop("]"); return arr
        if t.t == "IDENT":
            self.adv(); name = t.v
            if self.mop("("):
                args = []
                if not (self.peek().t == "OP" and self.peek().v == ")"):
                    args.append(self.expr())
                    while self.mop(","): args.append(self.expr())
                self.mop(")")
                return self.call(name, args)
            if self.mop("["):
                idx = self.expr(); self.mop("]")
                return self.env.get(name, [])[idx]
            return self.env.get(name)
        if self.mop("("):
            v = self.expr(); self.mop(")"); return v
        return None
    def unary(self):
        if self.mop("-"): return -self.unary()
        if self.mop("!"): return not self.unary()
        return self.primary()
    def mul(self):
        l = self.unary()
        while True:
            if self.mop("*"): l = l * self.unary()
            elif self.mop("/"): l = l / self.unary()
            elif self.mop("%"): l = l % self.unary()
            else: break
        return l
    def add(self):
        l = self.mul()
        while True:
            if self.mop("+"): l = l + self.mul()
            elif self.mop("-"): l = l - self.mul()
            else: break
        return l
    def cmp(self):
        l = self.add()
        for op, fn in (("==", lambda a,b: a==b),("!=",lambda a,b:a!=b),("<",lambda a,b:a<b),(">",lambda a,b:a>b),("<=",lambda a,b:a<=b),(">=",lambda a,b:a>=b)):
            if self.mop(op): return fn(l, self.add())
        return l
    def expr(self): return self.cmp()
    def skip_block(self):
        depth = 1
        while depth and self.peek().t != "EOF":
            t = self.adv()
            if t.t == "OP" and t.v == "{": depth += 1
            if t.t == "OP" and t.v == "}": depth -= 1
    def block(self):
        self.mop("{"); last = None
        while not (self.peek().t == "OP" and self.peek().v == "}") and self.peek().t != "EOF":
            if self.mkw("return"): return self.expr()
            last = self.stmt()
        self.mop("}"); return last
    def call(self, name, args):
        f = self.funcs.get(name)
        if not f: return None
        saved_env, saved_pos = self.env, self.pos
        frame = {p: (args[i] if i < len(args) else None) for i,p in enumerate(f["params"])}
        self.env = {**saved_env, **frame}
        self.pos = f["body"]
        ret = self.block()
        self.env, self.pos = saved_env, saved_pos
        return ret
    def stmt(self):
        if self.mkw("let"):
            name = self.adv().v; self.mop("="); val = self.expr(); self.env[name] = val; return val
        if self.mkw("print"):
            val = self.expr(); print(val); return val
        if self.mkw("while"):
            self.mop("("); cpos = self.pos; cond = self.expr(); self.mop(")"); bpos = self.pos
            guard = 0
            while cond and guard < 10000:
                self.pos = bpos; self.block()
                self.pos = cpos; cond = self.expr(); self.mop(")"); guard += 1
            self.pos = bpos
            if self.peek().t == "OP" and self.peek().v == "{":
                self.adv(); self.skip_block()
            return None
        if self.mkw("if"):
            self.mop("("); cond = self.expr(); self.mop(")")
            if cond:
                self.block()
                if self.mkw("else"):
                    if self.peek().t == "OP" and self.peek().v == "{": self.adv(); self.skip_block()
            else:
                if self.peek().t == "OP" and self.peek().v == "{": self.adv(); self.skip_block()
                if self.mkw("else"): self.block()
            return None
        if self.mkw("fn"):
            name = self.adv().v; self.mop("("); params = []
            if not (self.peek().t == "OP" and self.peek().v == ")"):
                params.append(self.adv().v)
                while self.mop(","): params.append(self.adv().v)
            self.mop(")"); body = self.pos
            self.funcs[name] = {"params": params, "body": body}
            if self.peek().t == "OP" and self.peek().v == "{": self.adv(); self.skip_block()
            return None
        if self.peek().t == "IDENT":
            save = self.pos; name = self.adv().v
            if self.mop("="):
                val = self.expr(); self.env[name] = val; return val
            self.pos = save
        return self.expr()
    def run(self):
        last = None
        while self.peek().t != "EOF": last = self.stmt()
        return last

if __name__ == "__main__":
    src = sys.argv[1] if len(sys.argv) > 1 else "let x = 6 * 7 print x"
    if src.endswith(".ns"): src = open(src).read()
    Mini(src).run()
