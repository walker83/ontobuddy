package cli

import (
	"fmt"
	"os"

	"github.com/walker/myonto/internal/rdf"
)

// LinkCmd 建立 <s> <p> <o> 三元组。
type LinkCmd struct {
	Subject   string `arg:"" required:"" placeholder:"SUBJECT" help:"主语（实体名、prefix:local 或完整 IRI）"`
	Predicate string `arg:"" required:"" placeholder:"PREDICATE" help:"谓词（关系名或 IRI），如 knows / bornIn / partOf"`
	Object    string `arg:"" required:"" placeholder:"OBJECT" help:"宾语（实体名、IRI，或加 -l 当作字面量）"`
	Label     string `help:"给谓词附一个 rdfs:label（用于自定义关系命名）" placeholder:"TEXT"`
	Literal   bool   `short:"l" help:"把宾语当作字面量（字符串）而非 IRI 实体"`
}

func (c *LinkCmd) Run() error {
	s, cfgPath, err := openStore()
	if err != nil {
		return err
	}
	subj, err := s.ResolveName(c.Subject)
	if err != nil {
		return fmt.Errorf("主语: %w", err)
	}
	pred, err := s.ResolveName(c.Predicate)
	if err != nil {
		return fmt.Errorf("谓词: %w", err)
	}
	var obj rdf.Term
	if c.Literal {
		obj = rdf.Lit(c.Object)
	} else {
		obj, err = s.ResolveName(c.Object)
		if err != nil {
			return fmt.Errorf("宾语: %w（如需字面量请加 -l）", err)
		}
	}

	t := rdf.Triple{Subject: subj, Predicate: pred, Object: obj}
	if s.Has(t) {
		fmt.Fprintln(os.Stdout, "（该三元组已存在）")
		return nil
	}
	s.Add(t)

	if c.Label != "" {
		// 给谓词本身打标签。
		s.Add(rdf.Triple{Subject: pred, Predicate: rdf.Label, Object: rdf.Lit(c.Label)})
	}

	if err := saveStore(s, cfgPath); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "已链接：%s %s %s\n",
		subj.LocalName(), pred.LocalName(), formatObject(obj))
	return nil
}

// UnlinkCmd 删除匹配的三元组。
type UnlinkCmd struct {
	Subject   string `arg:"" required:"" placeholder:"SUBJECT" help:"主语（实体名或 IRI）"`
	Predicate string `arg:"" required:"" placeholder:"PREDICATE" help:"谓词（关系名或 IRI）"`
	Object    string `arg:"" required:"" placeholder:"OBJECT" help:"宾语（与 -l / -a 配合使用）"`
	Literal   bool   `short:"l" help:"宾语按字面量匹配"`
	All       bool   `short:"a" help:"宾语通配：删除该主语+谓词的所有三元组"`
}

func (c *UnlinkCmd) Run() error {
	s, cfgPath, err := openStore()
	if err != nil {
		return err
	}
	subj, err := s.ResolveName(c.Subject)
	if err != nil {
		return fmt.Errorf("主语: %w", err)
	}
	pred, err := s.ResolveName(c.Predicate)
	if err != nil {
		return fmt.Errorf("谓词: %w", err)
	}
	var obj rdf.Term
	if c.All {
		// 留空作通配。
	} else if c.Literal {
		obj = rdf.Lit(c.Object)
	} else {
		obj, err = s.ResolveName(c.Object)
		if err != nil {
			return fmt.Errorf("宾语: %w（如需字面量匹配请加 -l）", err)
		}
	}

	n := s.Remove(rdf.Triple{Subject: subj, Predicate: pred, Object: obj})
	if n == 0 {
		fmt.Fprintln(os.Stdout, "（无匹配三元组）")
		return nil
	}
	if err := saveStore(s, cfgPath); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "已删除 %d 条三元组\n", n)
	return nil
}
