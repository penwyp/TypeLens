package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/penwyp/typelens/internal/service"
	"github.com/penwyp/typelens/pkg/typeless"
	"github.com/spf13/cobra"
)

func NewRootCommand() (*cobra.Command, error) {
	defaultConfig, err := service.DefaultConfig()
	if err != nil {
		return nil, err
	}

	svc := service.New(defaultConfig)
	cfg := defaultConfig
	root := &cobra.Command{
		Use:   "typelens",
		Short: "Typeless 词典与历史的桌面/命令行工具",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			svc.SetConfig(cfg)
		},
	}

	root.PersistentFlags().StringVar(&cfg.UserDataPath, "user-data", cfg.UserDataPath, "Typeless user-data.json 路径")
	root.PersistentFlags().StringVar(&cfg.DBPath, "db", cfg.DBPath, "Typeless typeless.db 路径")
	root.PersistentFlags().StringVar(&cfg.APIHost, "api-host", cfg.APIHost, "Typeless API 地址")
	root.PersistentFlags().IntVar(&cfg.TimeoutSec, "timeout", cfg.TimeoutSec, "HTTP 请求超时，单位秒")

	root.AddCommand(newDictCommand(svc))
	root.AddCommand(newHistoryCommand(svc))
	root.AddCommand(newAutoImportCommand(svc))
	return root, nil
}

func newDictCommand(svc *service.Service) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dict",
		Short: "管理 Typeless 词典",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "列出所有词条",
		RunE: func(cmd *cobra.Command, args []string) error {
			words, err := svc.ListDictionary(cmd.Context())
			if err != nil {
				return err
			}
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(words)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "add <term>",
		Short: "新增一个词条",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return svc.AddDictionaryTerm(cmd.Context(), args[0])
		},
	})

	var importDryRun bool
	var importConcurrency int
	importCmd := &cobra.Command{
		Use:   "import <file>",
		Short: "从文件导入词典",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := svc.ImportDictionary(cmd.Context(), service.ImportRequest{
				FilePath:    args[0],
				DryRun:      importDryRun,
				Concurrency: importConcurrency,
				LogWriter:   os.Stderr,
			})
			if err != nil {
				return err
			}
			action := "导入"
			if importDryRun {
				action = "将导入"
			}
			fmt.Printf("输入 %d 行，去重后 %d 个，跳过已有 %d 个，%s %d 个。\n",
				result.TotalInput,
				result.Unique,
				result.Skipped,
				action,
				result.Imported,
			)
			return nil
		},
	}
	importCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "只预览，不实际导入")
	importCmd.Flags().IntVar(&importConcurrency, "concurrency", 10, "导入并发数")
	cmd.AddCommand(importCmd)

	var deleteID string
	deleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "按词条 ID 删除",
		RunE: func(cmd *cobra.Command, args []string) error {
			return svc.DeleteDictionaryWord(cmd.Context(), deleteID)
		},
	}
	deleteCmd.Flags().StringVar(&deleteID, "id", "", "词条 ID")
	_ = deleteCmd.MarkFlagRequired("id")
	cmd.AddCommand(deleteCmd)

	var clearYes bool
	var clearConcurrency int
	clearCmd := &cobra.Command{
		Use:   "clear",
		Short: "清空词典",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !clearYes {
				return fmt.Errorf("清空词典需要显式传入 --yes")
			}
			deleted, err := svc.ClearDictionary(cmd.Context(), service.ClearRequest{
				Concurrency: clearConcurrency,
				LogWriter:   os.Stderr,
			})
			if err != nil {
				return err
			}
			fmt.Printf("已删除 %d 个词。\n", deleted)
			return nil
		},
	}
	clearCmd.Flags().BoolVar(&clearYes, "yes", false, "确认执行清空操作")
	clearCmd.Flags().IntVar(&clearConcurrency, "concurrency", 10, "清空时删除并发数")
	cmd.AddCommand(clearCmd)

	var resetYes bool
	var resetFile string
	var resetConcurrency int
	resetCmd := &cobra.Command{
		Use:   "reset",
		Short: "重置词典为默认词",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !resetYes {
				return fmt.Errorf("重置词典需要显式传入 --yes")
			}
			result, err := svc.ResetDictionary(cmd.Context(), service.ResetRequest{
				DefaultsFile: resetFile,
				Concurrency:  resetConcurrency,
				LogWriter:    os.Stderr,
			})
			if err != nil {
				return err
			}
			fmt.Printf("目标 %d 个，保留 %d 个，删除 %d 个，新增 %d 个。\n",
				result.Unique,
				result.Kept,
				result.Deleted,
				result.Imported,
			)
			return nil
		},
	}
	resetCmd.Flags().BoolVar(&resetYes, "yes", false, "确认执行重置操作")
	resetCmd.Flags().StringVar(&resetFile, "file", "", "默认词文件；为空时使用内置默认词")
	resetCmd.Flags().IntVar(&resetConcurrency, "concurrency", 10, "重置时删除/新增的并发数")
	cmd.AddCommand(resetCmd)

	return cmd
}

func newHistoryCommand(svc *service.Service) *cobra.Command {
	var (
		limit       int
		keyword     string
		regexExpr   string
		contextMode string
		noCopy      bool
		full        bool
	)

	cmd := &cobra.Command{
		Use:   "history",
		Short: "查询 Typeless 历史并复制最新一条",
		RunE: func(cmd *cobra.Command, args []string) error {
			records, err := svc.QueryHistory(cmd.Context(), service.HistoryQuery{
				Limit:       limit,
				Keyword:     keyword,
				Regex:       regexExpr,
				ContextMode: contextMode,
			})
			if err != nil {
				return err
			}
			if len(records) == 0 {
				fmt.Println("没有匹配的历史记录。")
				return nil
			}
			printTranscriptRecords(records, full)
			if !noCopy {
				if err := svc.CopyText(cmd.Context(), records[0].Text); err != nil {
					return err
				}
				fmt.Println("\n已复制最新一条到剪贴板。")
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 20, "列出最近 N 条记录")
	cmd.Flags().StringVar(&keyword, "keyword", "", "按关键字大小写不敏感过滤")
	cmd.Flags().StringVar(&regexExpr, "regex", "", "按正则表达式过滤")
	cmd.Flags().StringVar(&contextMode, "context", "frontmost", "上下文来源: frontmost/latest/all")
	cmd.Flags().BoolVar(&noCopy, "no-copy", false, "不复制最新记录到剪贴板")
	cmd.Flags().BoolVar(&full, "full", false, "输出完整转写文本")
	return cmd
}

func printTranscriptRecords(records []typeless.TranscriptRecord, full bool) {
	for index, record := range records {
		text := record.Text
		if !full {
			text = typeless.OneLine(text, 120)
		}
		fmt.Printf("[%d] %s\n%s\n", index+1, record.CreatedAt, strings.TrimSpace(text))
	}
}
