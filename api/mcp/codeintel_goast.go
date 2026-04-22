package mcp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
)

// parseGoAST uses the Go AST package to extract symbols, call edges, and imports
// from a Go source file. This provides accurate structural analysis for Go code
// without requiring Tree-sitter binaries.
func parseGoAST(path, rootPath string) ([]SymbolInfo, []symbolEdge, []importEdge) {
	relPath, _ := filepath.Rel(rootPath, path)
	if relPath == "" {
		relPath = path
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, nil, nil
	}

	pkgName := ""
	if f.Name != nil {
		pkgName = f.Name.Name
	}

	var symbols []SymbolInfo
	var edges []symbolEdge
	var imports []importEdge

	// Extract imports.
	for _, imp := range f.Imports {
		if imp.Path != nil {
			pkg := strings.Trim(imp.Path.Value, "\"")
			imports = append(imports, importEdge{file: relPath, pkg: pkg})
		}
	}

	// Extract top-level declarations and build symbol table.
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			sym := goFuncToSymbol(d, fset, relPath, pkgName)
			if sym != nil {
				symbols = append(symbols, *sym)
			}

		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					sym := goTypeToSymbol(s, fset, relPath, pkgName)
					if sym != nil {
						symbols = append(symbols, *sym)
					}
				}
			}
		}
	}

	// Build symbol ID lookup for this file.
	localSymbols := make(map[string]string) // name -> ID
	for _, sym := range symbols {
		localSymbols[sym.Name] = sym.ID
	}

	// Extract call edges from function bodies.
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		callerName := fn.Name.Name
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			// Method: use receiver type prefix.
			callerName = fn.Name.Name
		}
		callerID := relPath + ":" + callerName

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			calleeName := resolveCallName(call)
			if calleeName == "" {
				return true
			}

			// Try to resolve to a local symbol.
			calleeID := ""
			if id, ok := localSymbols[calleeName]; ok {
				calleeID = id
			} else {
				// Cross-file call; use a qualified reference.
				calleeID = calleeName
			}

			pos := fset.Position(call.Pos())
			edges = append(edges, symbolEdge{
				callerID: callerID,
				calleeID: calleeID,
				file:     relPath,
				line:     pos.Line,
			})
			return true
		})
	}

	return symbols, edges, imports
}

// goFuncToSymbol converts a Go function declaration to a SymbolInfo.
func goFuncToSymbol(fn *ast.FuncDecl, fset *token.FileSet, file, pkg string) *SymbolInfo {
	if fn.Name == nil {
		return nil
	}
	name := fn.Name.Name
	kind := "function"
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		kind = "method"
	}
	visibility := "public"
	if len(name) > 0 && name[0] >= 'a' && name[0] <= 'z' {
		visibility = "private"
	}

	pos := fset.Position(fn.Pos())

	// Build a basic signature.
	sig := "func " + name + "("
	if fn.Type.Params != nil {
		var params []string
		for _, field := range fn.Type.Params.List {
			typeName := typeExprString(field.Type)
			if len(field.Names) == 0 {
				params = append(params, typeName)
			} else {
				for _, n := range field.Names {
					params = append(params, n.Name+" "+typeName)
				}
			}
		}
		sig += strings.Join(params, ", ")
	}
	sig += ")"
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		var results []string
		for _, field := range fn.Type.Results.List {
			results = append(results, typeExprString(field.Type))
		}
		if len(results) == 1 {
			sig += " " + results[0]
		} else {
			sig += " (" + strings.Join(results, ", ") + ")"
		}
	}

	// Extract doc comment.
	var doc string
	if fn.Doc != nil {
		doc = strings.TrimSpace(fn.Doc.Text())
	}

	return &SymbolInfo{
		ID:         file + ":" + name,
		Name:       name,
		Kind:       kind,
		File:       file,
		Line:       pos.Line,
		Language:   "go",
		Package:    pkg,
		Visibility: visibility,
		Signature:  sig,
		DocComment: doc,
	}
}

// goTypeToSymbol converts a Go type declaration to a SymbolInfo.
func goTypeToSymbol(spec *ast.TypeSpec, fset *token.FileSet, file, pkg string) *SymbolInfo {
	if spec.Name == nil {
		return nil
	}
	name := spec.Name.Name
	kind := "type"
	if _, ok := spec.Type.(*ast.InterfaceType); ok {
		kind = "interface"
	}
	visibility := "public"
	if len(name) > 0 && name[0] >= 'a' && name[0] <= 'z' {
		visibility = "private"
	}
	pos := fset.Position(spec.Pos())
	return &SymbolInfo{
		ID:         file + ":" + name,
		Name:       name,
		Kind:       kind,
		File:       file,
		Line:       pos.Line,
		Language:   "go",
		Package:    pkg,
		Visibility: visibility,
	}
}

// resolveCallName extracts the function name from a call expression.
func resolveCallName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		// pkg.Func or receiver.Method
		if ident, ok := fn.X.(*ast.Ident); ok {
			return ident.Name + "." + fn.Sel.Name
		}
		return fn.Sel.Name
	}
	return ""
}

// typeExprString returns a human-readable string for a type expression.
func typeExprString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeExprString(t.X)
	case *ast.SelectorExpr:
		return typeExprString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		return "[]" + typeExprString(t.Elt)
	case *ast.MapType:
		return "map[" + typeExprString(t.Key) + "]" + typeExprString(t.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.Ellipsis:
		return "..." + typeExprString(t.Elt)
	case *ast.FuncType:
		return "func(...)"
	case *ast.ChanType:
		return "chan " + typeExprString(t.Value)
	default:
		return "any"
	}
}
