package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/chromedp"
)

type ChatPair struct {
	Index    int
	Question string
	Answer   string
}

func main() {
	// urls.csv 파일 읽기
	csvFile, err := os.Open("urls.csv")
	if err != nil {
		if os.IsNotExist(err) {
			// urls.csv 템플릿 생성
			f, createErr := os.Create("urls.csv")
			if createErr == nil {
				f.WriteString("https://share.gemini.google/zv9ENNwoXrnh\n")
				f.Close()
				fmt.Println("urls.csv 파일이 존재하지 않아 새로 생성했습니다. urls.csv 파일에 공유 URL들을 입력한 후 다시 실행해주세요.")
				os.Exit(0)
			}
		}
		log.Fatalf("urls.csv 파일을 열 수 없습니다: %v", err)
	}
	defer csvFile.Close()

	reader := csv.NewReader(csvFile)
	records, err := reader.ReadAll()
	if err != nil {
		log.Fatalf("urls.csv 읽기 실패: %v", err)
	}

	var urls []string
	for _, record := range records {
		if len(record) > 0 {
			url := strings.TrimSpace(record[0])
			if url != "" && (strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")) {
				urls = append(urls, url)
			}
		}
	}

	if len(urls) == 0 {
		fmt.Println("urls.csv 파일에 유효한 URL이 존재하지 않습니다. 파일을 확인해주세요.")
		os.Exit(0)
	}

	// 1. chromedp 브라우저 Context 생성
	browserCtx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	// 브라우저 최초 구동을 위한 더미 호출 (백그라운드 크롬 엔진 활성화)
	if err := chromedp.Run(browserCtx); err != nil {
		log.Fatalf("chromedp 초기화 실패: %v", err)
	}

	fmt.Printf("총 %d개의 URL 파싱을 시작합니다 (동시 처리 적용).\n\n", len(urls))

	var wg sync.WaitGroup
	// 동시 처리 제한 (세마포어 채널 - 최대 3개 동시 실행)
	maxWorkers := 3
	if len(urls) < maxWorkers {
		maxWorkers = len(urls)
	}
	sem := make(chan struct{}, maxWorkers)

	for i, shareURL := range urls {
		wg.Add(1)
		sem <- struct{}{} // 세마포어 점유

		go func(idx int, url string) {
			defer wg.Done()
			defer func() { <-sem }() // 세마포어 해제

			fmt.Printf("[%d/%d] URL 처리 시작: %s\n", idx, len(urls), url)
			err := processURL(browserCtx, url, idx)
			if err != nil {
				fmt.Printf("[%d/%d] 에러 발생: %v\n\n", idx, len(urls), err)
			}
		}(i+1, shareURL)
	}

	wg.Wait()
	fmt.Println("모든 URL 파싱 작업이 완료되었습니다.")
}

func processURL(browserCtx context.Context, shareURL string, index int) error {
	// 각 URL 처리를 위한 개별 탭(자식 컨텍스트) 생성
	ctx, cancelTab := chromedp.NewContext(browserCtx)
	defer cancelTab()

	// 탭에 대한 개별 타임아웃 지정
	ctx, cancelTimeout := context.WithTimeout(ctx, 45*time.Second)
	defer cancelTimeout()

	var htmlContent string
	var initialDataRaw string

	// 2. 브라우저 액션 실행
	err := chromedp.Run(ctx,
		chromedp.Navigate(shareURL),
		chromedp.Sleep(5*time.Second), // 리소스 및 JS 변수 완전 초기화 대기
		chromedp.OuterHTML("html", &htmlContent),
		chromedp.Evaluate(`
			(function() {
				try {
					if (window.WIZ_global_data) {
						return JSON.stringify(window.WIZ_global_data);
					}
					var keys = Object.keys(window);
					for (var i=0; i<keys.length; i++) {
						if (keys[i].indexOf('INITIAL_DATA') !== -1 || keys[i].indexOf('global_data') !== -1) {
							return JSON.stringify(window[keys[i]]);
						}
					}
				} catch(e) {}
				return "";
			})()
		`, &initialDataRaw),
	)
	if err != nil {
		return fmt.Errorf("페이지 렌더링 또는 스크립트 실행 실패: %w", err)
	}

	// 3. goquery를 사용하여 렌더링된 HTML DOM 파싱
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return fmt.Errorf("DOM 파싱 에러: %w", err)
	}

	// 4. 대화방 기본 정보 추출 (제목, 모델, 공유 시간)
	chatTitle := "제목 없음"
	modelName := "확인 불가"
	shareTime := "확인 불가"

	// 제목 후보 셀렉터들
	doc.Find("h1, .conversation-title, .title, [class*=\"title\"]").Each(func(i int, s *goquery.Selection) {
		t := strings.TrimSpace(s.Text())
		if t != "" && chatTitle == "제목 없음" && !strings.Contains(strings.ToLower(t), "gemini") {
			chatTitle = t
		}
	})
	if chatTitle == "제목 없음" {
		t := strings.TrimSpace(doc.Find("title").Text())
		if t != "" {
			chatTitle = strings.Replace(t, " - Gemini", "", -1)
		}
	}

	doc.Find("div, span, p").Each(func(i int, s *goquery.Selection) {
		classStr, _ := s.Attr("class")
		text := strings.TrimSpace(s.Text())
		if strings.Contains(classStr, "model") && text != "" && modelName == "확인 불가" {
			modelName = text
		}
		if (strings.Contains(classStr, "timestamp") || strings.Contains(classStr, "time")) && text != "" && shareTime == "확인 불가" {
			shareTime = text
		}
	})

	// 5. 대화 턴(Turn)별 DOM 추출 및 질문-답변 매핑
	var chatPairs []ChatPair
	var currentPair *ChatPair
	pairIndex := 1

	// 질문(User)과 답변(Gemini) 요소들을 순서대로 탐색하기 위한 통합 선택자
	selector := ".query-text, .user-query, .markdown-content, x-gemini-markdown, .message-content"
	doc.Find(selector).Each(func(i int, s *goquery.Selection) {
		// 불필요한 스크린 리더용 텍스트 제거 (예: "말씀하신 내용:")
		s.Find(".sr-only, [class*=\"sr-only\"], .cdk-visually-hidden, [class*=\"cdk-visually-hidden\"]").Remove()

		content := strings.TrimSpace(s.Text())
		if content == "" {
			return
		}

		isUser := s.HasClass("query-text") || s.HasClass("user-query")

		if isUser {
			// 이미 이전 질문 페어가 완료되지 않은 상태에서 새로운 질문이 왔을 경우 이전 질문 저장
			if currentPair != nil {
				chatPairs = append(chatPairs, *currentPair)
			}
			currentPair = &ChatPair{
				Index:    pairIndex,
				Question: content,
			}
			pairIndex++
		} else {
			// Gemini 답변인 경우
			if currentPair != nil {
				if currentPair.Answer != "" {
					// 혹시 모를 연속된 답변인 경우 개행 처리하여 합침
					currentPair.Answer += "\n\n" + content
				} else {
					currentPair.Answer = content
				}
			} else {
				// 질문이 없는데 답변이 먼저 나온 예외 케이스 처리
				currentPair = &ChatPair{
					Index:  pairIndex,
					Answer: content,
				}
				pairIndex++
			}
		}
	})

	// 마지막 남은 페어 저장
	if currentPair != nil {
		chatPairs = append(chatPairs, *currentPair)
	}

	// 파일명 생성을 위한 제목 정제 (특수문자 제거 및 길이 제한)
	safeTitle := chatTitle
	// 파일명 금지 문자 제거
	reg, _ := regexp.Compile(`[\\/:*?"<>|]`)
	safeTitle = reg.ReplaceAllString(safeTitle, "")
	safeTitle = strings.TrimSpace(safeTitle)
	safeTitle = strings.ReplaceAll(safeTitle, " ", "_")

	// 제목 길이 제한 (최대 20자)
	runes := []rune(safeTitle)
	if len(runes) > 20 {
		safeTitle = string(runes[:20])
	}
	if safeTitle == "" {
		safeTitle = "제목없음"
	}

	csvFilePath := fmt.Sprintf("%d_%s.csv", index, safeTitle)
	csvFile, err := os.Create(csvFilePath)
	if err != nil {
		return fmt.Errorf("CSV 생성 실패: %w", err)
	}
	defer csvFile.Close()

	csvFile.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(csvFile)
	defer writer.Flush()

	// 사용모드, 대화 시작일, 링크 게시일 정보 정밀 분리
	modelName = "확인 불가"
	chatStartTime := "확인 불가"
	sharePublishTime := "확인 불가"

	// 날짜 정규식 매칭 (예: 2026년 7월 11일 오후 01:37)
	dateReg := regexp.MustCompile(`\d{4}년\s*\d{1,2}월\s*\d{1,2}일\s*(오전|오후)\s*\d{1,2}:\d{2}`)
	dateMatches := dateReg.FindAllString(shareTime, -1)

	if len(dateMatches) >= 2 {
		chatStartTime = dateMatches[0]
		sharePublishTime = dateMatches[1]

		firstMatchIdx := strings.Index(shareTime, dateMatches[0])
		if firstMatchIdx > 0 {
			modelPart := strings.TrimSpace(shareTime[:firstMatchIdx])
			modelPart = strings.TrimPrefix(modelPart, "사용 모드:")
			modelPart = strings.TrimSpace(modelPart)
			if modelPart != "" {
				modelName = modelPart
			}
		}
	} else if len(dateMatches) == 1 {
		chatStartTime = dateMatches[0]

		firstMatchIdx := strings.Index(shareTime, dateMatches[0])
		if firstMatchIdx > 0 {
			modelPart := strings.TrimSpace(shareTime[:firstMatchIdx])
			modelPart = strings.TrimPrefix(modelPart, "사용 모드:")
			modelPart = strings.TrimSpace(modelPart)
			if modelPart != "" {
				modelName = modelPart
			}
		}
	} else {
		// 날짜 형식이 검출되지 않은 경우 원본 텍스트를 대안으로 보존
		if shareTime != "" && shareTime != "확인 불가" {
			modelName = shareTime
		}
	}

	writer.Write([]string{"[대화 메타데이터]", ""})
	writer.Write([]string{"대화 제목", chatTitle})
	writer.Write([]string{"공유 URL", shareURL})
	writer.Write([]string{"마지막 사용 모델", modelName})
	writer.Write([]string{"대화 시작일", chatStartTime})
	writer.Write([]string{"링크 게시일", sharePublishTime})
	writer.Write([]string{"", ""})

	writer.Write([]string{"No.", "질문", "대답"})

	fmt.Printf("--- [%d] 분석 결과 요약 ---\n", index)
	fmt.Printf("대화 제목: %s\n", chatTitle)
	fmt.Printf("사용 모델: %s\n", modelName)
	fmt.Printf("대화 시작일: %s\n", chatStartTime)
	fmt.Printf("링크 게시일: %s\n", sharePublishTime)

	for _, pair := range chatPairs {
		qClean := strings.ReplaceAll(pair.Question, "\r\n", "\n")
		aClean := strings.ReplaceAll(pair.Answer, "\r\n", "\n")
		writer.Write([]string{
			strconv.Itoa(pair.Index),
			qClean,
			aClean,
		})
	}

	fmt.Printf("결과 저장 완료: %s\n\n", csvFilePath)
	return nil
}
