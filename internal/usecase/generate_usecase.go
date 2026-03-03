package usecase

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ai-news/internal/domain"
	"ai-news/internal/infra/audio"
	"ai-news/internal/infra/gemini"
	"ai-news/internal/infra/navidrome"
	"ai-news/internal/infra/storage"
	"ai-news/internal/infra/voicevox"
	"ai-news/internal/repository"
)

// GenerateUsecase implements Stages 2–7 of the pipeline:
// Gemini selection → VOICEVOX TTS → ffmpeg encode → SMB save → digest concat → Navidrome scan.
type GenerateUsecase struct {
	running atomic.Bool

	pipelineRepo  *repository.PipelineRepo
	settingsRepo  *repository.SettingsRepo
	categoryRepo  *repository.CategoryRepo
	articleRepo   *repository.ArticleRepo
	broadcastRepo *repository.BroadcastRepo

	geminiAPIKey         string
	maxGeminiConcurrency int

	voicevoxClient *voicevox.Client
	musicStore     *storage.MusicStore
	naviClient     *navidrome.Client
	appBaseURL     string
}

// NewGenerateUsecase creates a GenerateUsecase.
func NewGenerateUsecase(
	pr *repository.PipelineRepo,
	sr *repository.SettingsRepo,
	cr *repository.CategoryRepo,
	ar *repository.ArticleRepo,
	br *repository.BroadcastRepo,
	geminiAPIKey string,
	maxGeminiConcurrency int,
	vc *voicevox.Client,
	ms *storage.MusicStore,
	nc *navidrome.Client,
	appBaseURL string,
) *GenerateUsecase {
	return &GenerateUsecase{
		pipelineRepo:         pr,
		settingsRepo:         sr,
		categoryRepo:         cr,
		articleRepo:          ar,
		broadcastRepo:        br,
		geminiAPIKey:         geminiAPIKey,
		maxGeminiConcurrency: maxGeminiConcurrency,
		voicevoxClient:       vc,
		musicStore:           ms,
		naviClient:           nc,
		appBaseURL:           appBaseURL,
	}
}

// IsRunning reports whether a generate job is currently in progress.
func (uc *GenerateUsecase) IsRunning() bool { return uc.running.Load() }

// Run executes Stages 2–7 of the pipeline. triggeredBy is "cron", "api", or "ui".
// Returns domain.ErrPipelineActive if a run is already in progress.
func (uc *GenerateUsecase) Run(ctx context.Context, triggeredBy string) error {
	if !uc.running.CompareAndSwap(false, true) {
		return domain.ErrPipelineActive
	}
	defer uc.running.Store(false)

	runID, err := uc.pipelineRepo.Create(ctx, &domain.PipelineRun{
		JobType:     domain.JobTypeGenerate,
		Status:      domain.RunStatusRunning,
		TriggeredBy: domain.TriggeredBy(triggeredBy),
	})
	if err != nil {
		return fmt.Errorf("create pipeline run: %w", err)
	}

	genErr := uc.runGenerate(ctx, runID)

	status := domain.RunStatusCompleted
	var errMsg *string
	if genErr != nil {
		status = domain.RunStatusFailed
		msg := genErr.Error()
		errMsg = &msg
	}
	if err := uc.pipelineRepo.Finish(ctx, runID, status, errMsg); err != nil {
		log.Printf("generate: finish pipeline run: %v", err)
	}
	return genErr
}

func (uc *GenerateUsecase) runGenerate(ctx context.Context, runID int64) error {
	settings, err := uc.settingsRepo.Get(ctx)
	if err != nil {
		return fmt.Errorf("get settings: %w", err)
	}

	categories, err := uc.categoryRepo.ListEnabled(ctx)
	if err != nil {
		return fmt.Errorf("list categories: %w", err)
	}

	// Count unprocessed articles for voicevox categories.
	totalUnprocessed := 0
	for _, cat := range categories {
		if cat.TTSEngine != "voicevox" {
			continue
		}
		n, err := uc.articleRepo.CountUnprocessed(ctx, cat.Category)
		if err != nil {
			return fmt.Errorf("count unprocessed (%s): %w", cat.Category, err)
		}
		totalUnprocessed += n
	}
	if totalUnprocessed == 0 {
		log.Printf("generate: no unprocessed articles, recording no-op")
		_ = uc.pipelineRepo.UpdateSelected(ctx, runID, 0)
		return nil
	}

	// Stage 2: Gemini selection.
	_ = uc.pipelineRepo.UpdateStep(ctx, runID, "gemini_selection")
	gc := gemini.New(uc.geminiAPIKey, settings.GeminiModel)
	totalSelected, err := uc.runGeminiSelection(ctx, categories, settings, gc)
	if err != nil {
		return fmt.Errorf("gemini selection: %w", err)
	}
	_ = uc.pipelineRepo.UpdateSelected(ctx, runID, totalSelected)
	if totalSelected == 0 {
		return nil
	}

	// Stages 3–5: TTS + encode + save per category (sort_order).
	_ = uc.pipelineRepo.UpdateStep(ctx, runID, "tts_encode_save")
	episodePaths, err := uc.runEpisodes(ctx, runID, categories, settings)
	if err != nil {
		return err
	}

	// Stage 6: Digest (non-fatal).
	_ = uc.pipelineRepo.UpdateStep(ctx, runID, "digest")
	if err := uc.runDigest(ctx, runID, categories, episodePaths); err != nil {
		log.Printf("generate: digest (non-fatal): %v", err)
	}

	// Stage 7: Navidrome scan (non-fatal).
	_ = uc.pipelineRepo.UpdateStep(ctx, runID, "navidrome_scan")
	if uc.naviClient != nil {
		if err := uc.naviClient.StartScan(ctx); err != nil {
			log.Printf("generate: navidrome scan (non-fatal): %v", err)
		}
	}

	_ = uc.pipelineRepo.UpdateStep(ctx, runID, "completed")
	return nil
}

func (uc *GenerateUsecase) runGeminiSelection(
	ctx context.Context,
	categories []*domain.CategorySettings,
	settings *domain.AppSettings,
	gc *gemini.Client,
) (int, error) {
	sem := make(chan struct{}, uc.maxGeminiConcurrency)
	type result struct {
		count int
		err   error
	}
	resultCh := make(chan result, len(categories))
	var wg sync.WaitGroup

	for _, cat := range categories {
		if cat.TTSEngine != "voicevox" {
			continue
		}
		cat := cat
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			n, err := uc.selectForCategory(ctx, gc, cat, settings)
			resultCh <- result{count: n, err: err}
		}()
	}
	wg.Wait()
	close(resultCh)

	total := 0
	for r := range resultCh {
		if r.err != nil {
			return 0, r.err
		}
		total += r.count
	}
	return total, nil
}

func (uc *GenerateUsecase) selectForCategory(
	ctx context.Context,
	gc *gemini.Client,
	cat *domain.CategorySettings,
	_ *domain.AppSettings,
) (int, error) {
	articles, err := uc.articleRepo.ListUnprocessed(ctx, cat.Category, cat.ArticlesPerEpisode*3)
	if err != nil {
		return 0, fmt.Errorf("list unprocessed (%s): %w", cat.Category, err)
	}
	if len(articles) == 0 {
		return 0, nil
	}

	sel, err := gc.SelectAndSummarize(ctx, articles, cat, cat.ArticlesPerEpisode, cat.SummaryCharsPerArticle)
	if err != nil {
		return 0, fmt.Errorf("gemini (%s): %w", cat.Category, err)
	}

	count := 0
	for _, a := range articles {
		rank, ok := sel.Ranks[a.ID]
		if !ok {
			continue
		}
		if err := uc.articleRepo.MarkSelected(ctx, a.ID, rank, sel.Summaries[a.ID]); err != nil {
			log.Printf("generate: mark selected article %d: %v", a.ID, err)
		}
		count++
	}
	log.Printf("generate: category=%s selected=%d/%d", cat.Category, count, len(articles))
	return count, nil
}

func (uc *GenerateUsecase) runEpisodes(
	ctx context.Context,
	runID int64,
	categories []*domain.CategorySettings,
	settings *domain.AppSettings,
) (map[string]string, error) {
	paths := make(map[string]string)
	for _, cat := range categories {
		if cat.TTSEngine != "voicevox" {
			log.Printf("generate: category=%s TTSEngine=%s (unsupported), skipping", cat.Category, cat.TTSEngine)
			continue
		}
		selected, err := uc.articleRepo.ListSelected(ctx, cat.Category)
		if err != nil {
			return nil, fmt.Errorf("list selected (%s): %w", cat.Category, err)
		}
		if len(selected) == 0 {
			continue
		}
		fp, err := uc.generateEpisode(ctx, runID, cat, selected, settings)
		if err != nil {
			return nil, fmt.Errorf("episode (%s): %w", cat.Category, err)
		}
		paths[cat.Category] = fp
	}
	return paths, nil
}

func (uc *GenerateUsecase) generateEpisode(
	ctx context.Context,
	runID int64,
	cat *domain.CategorySettings,
	articles []*domain.Article,
	settings *domain.AppSettings,
) (string, error) {
	now := time.Now()
	title := fmt.Sprintf("%s - %s %s", cat.DisplayName, now.Format("2006-01-02"), timePeriodJA(now.Hour()))

	var wavSlices [][]byte

	introText := fmt.Sprintf("こんにちは。AIニュースステーションです。%sのニュースをお届けします。", title)
	introWAV, err := uc.voicevoxClient.Synthesize(ctx, introText, cat.VoicevoxSpeakerID, settings.VoicevoxSpeedScale)
	if err != nil {
		return "", fmt.Errorf("tts intro: %w", err)
	}
	wavSlices = append(wavSlices, introWAV)

	var scriptParts []string
	for _, a := range articles {
		text := a.Title
		if a.Summary != nil && *a.Summary != "" {
			text = *a.Summary
		}
		wav, err := uc.voicevoxClient.Synthesize(ctx, text, cat.VoicevoxSpeakerID, settings.VoicevoxSpeedScale)
		if err != nil {
			return "", fmt.Errorf("tts article %d: %w", a.ID, err)
		}
		wavSlices = append(wavSlices, wav)
		scriptParts = append(scriptParts, text)
	}

	outroWAV, err := uc.voicevoxClient.Synthesize(ctx, "以上、本日のニュースでした。またお聞きください。",
		cat.VoicevoxSpeakerID, settings.VoicevoxSpeedScale)
	if err != nil {
		return "", fmt.Errorf("tts outro: %w", err)
	}
	wavSlices = append(wavSlices, outroWAV)

	encoded, err := audio.WAVsToMP3(ctx, wavSlices, title)
	if err != nil {
		return "", fmt.Errorf("encode mp3: %w", err)
	}

	filename := fmt.Sprintf("%s-%s.mp3", cat.Category, now.Format("20060102-150405"))
	filePath := uc.musicStore.SMBPath(cat.Category, filename)
	if err := uc.musicStore.SaveMP3(filePath, encoded.Data); err != nil {
		return "", fmt.Errorf("save mp3: %w", err)
	}

	fullScript := strings.Join(scriptParts, "\n\n")
	dur := encoded.DurationSec
	broadcastID, err := uc.broadcastRepo.Create(ctx, &domain.Broadcast{
		PipelineRunID: runID,
		Category:      cat.Category,
		BroadcastType: domain.BroadcastTypeEpisode,
		Title:         title,
		Script:        &fullScript,
		FilePath:      filePath,
		DurationSec:   &dur,
	})
	if err != nil {
		return "", fmt.Errorf("create broadcast: %w", err)
	}

	fileURL := fmt.Sprintf("%s/media/%s/%d", uc.appBaseURL, cat.Category, broadcastID)
	_ = uc.broadcastRepo.SetFileURL(ctx, broadcastID, fileURL)

	for _, a := range articles {
		if err := uc.articleRepo.SetBroadcastID(ctx, a.ID, broadcastID); err != nil {
			log.Printf("generate: set broadcast_id article %d: %v", a.ID, err)
		}
	}

	log.Printf("generate: episode category=%s file=%s duration=%ds", cat.Category, filePath, dur)
	return filePath, nil
}

func (uc *GenerateUsecase) runDigest(
	ctx context.Context,
	runID int64,
	categories []*domain.CategorySettings,
	episodePaths map[string]string,
) error {
	if len(episodePaths) == 0 {
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "ai-news-digest-*")
	if err != nil {
		return fmt.Errorf("mktempdir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	var mp3Slices [][]byte
	for _, cat := range categories {
		fp, ok := episodePaths[cat.Category]
		if !ok {
			continue
		}
		data, err := uc.musicStore.ReadMP3(fp)
		if err != nil {
			return fmt.Errorf("read episode (%s): %w", cat.Category, err)
		}
		mp3Slices = append(mp3Slices, data)
	}
	if len(mp3Slices) == 0 {
		return nil
	}

	digestData, err := audio.ConcatMP3(ctx, mp3Slices, tmpDir)
	if err != nil {
		return fmt.Errorf("concat mp3: %w", err)
	}

	// Measure duration.
	measurePath := tmpDir + "/measure.mp3"
	if writeErr := os.WriteFile(measurePath, digestData, 0600); writeErr == nil {
		// ignore probe errors — duration stays 0
	}
	dur, _ := audio.ProbeDuration(ctx, measurePath)

	now := time.Now()
	digestTitle := fmt.Sprintf("ダイジェスト - %s %s", now.Format("2006-01-02"), timePeriodJA(now.Hour()))
	filename := fmt.Sprintf("digest-%s.mp3", now.Format("20060102-150405"))
	filePath := uc.musicStore.SMBPath("digest", filename)
	if err := uc.musicStore.SaveMP3(filePath, digestData); err != nil {
		return fmt.Errorf("save digest: %w", err)
	}

	broadcastID, err := uc.broadcastRepo.Create(ctx, &domain.Broadcast{
		PipelineRunID: runID,
		Category:      "digest",
		BroadcastType: domain.BroadcastTypeDigest,
		Title:         digestTitle,
		FilePath:      filePath,
		DurationSec:   &dur,
	})
	if err != nil {
		return fmt.Errorf("create digest broadcast: %w", err)
	}

	fileURL := fmt.Sprintf("%s/media/digest/%d", uc.appBaseURL, broadcastID)
	_ = uc.broadcastRepo.SetFileURL(ctx, broadcastID, fileURL)

	log.Printf("generate: digest file=%s duration=%ds", filePath, dur)
	return nil
}

// timePeriodJA maps an hour (0–23) to a Japanese time-period label.
func timePeriodJA(hour int) string {
	switch {
	case hour < 6:
		return "深夜"
	case hour < 11:
		return "朝"
	case hour < 16:
		return "昼"
	case hour < 21:
		return "夜"
	default:
		return "深夜"
	}
}
