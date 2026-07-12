# Gemini 채팅 공유 대화 파서

구글 Gemini 채팅의 **대화 공유 링크** 목록을 기반으로 대화내용을 자동으로 스크롤 및 렌더링하여 CSV 형태로 일괄 추출해 주는 Go 언어 기반 파서 도구입니다.

---

## 주요 기능

개별 URL을 인자로 넘길 필요 없이, 실행 디렉토리의 `urls.csv` 파일에 추출할 Gemini 공유 URL들을 적어두면 파싱하여 대화별 CSV파일로 추출합니다.

---

## 실행환경

- **Go SDK**: Go 1.19 버전 이상(실행파일을 실행하는 경우 필요 없음)
- **웹 브라우저**: 시스템에 Google Chrome 브라우저가 설치되어 있어야 합니다.

---

## 실행 방법 (OS별)

이미 OS별 컴파일이 완료된 실행 파일이 `build/` 디렉토리에 포함되어 있습니다. 실행 환경에 맞는 바이너리를 실행해야 합니다.

- **macOS (Apple Silicon M1/M2/M3)**: `build/gemini_parser_mac_arm64`
- **macOS (Intel CPU)**: `build/gemini_parser_mac_amd64`
- **Windows (64bit)**: `build/gemini_parser_win.exe`
- **Linux (64bit)**: `build/gemini_parser_linux`

### 1. URL 입력 파일 준비 (`urls.csv`)

실행 바이너리가 위치한 폴더에 `urls.csv` 파일을 만들고, 세로로 줄당 하나씩 Gemini 공유 URL을 적어줍니다.

_예시 (`urls.csv` 내용)_:

```csv
https://share.gemini.google/test_url1
https://share.gemini.google/test_url2
```

> **Note**  
> 만약 실행 파일 위치에 `urls.csv` 파일이 존재하지 않는 상태에서 프로그램을 실행하면, 기본 예시 주소가 들어있는 `urls.csv` 템플릿 파일이 자동으로 생성됩니다.

### 2. 프로그램 실행

- **macOS / Linux**:

  ```bash
  # 권한 부여 (최초 1회)
  chmod +x ./gemini_parser_mac_arm64

  # 실행
  ./gemini_parser_mac_arm64
  ```

- **Windows**:
  - `gemini_parser_win.exe` 파일이 있는 위치의 파일 탐색기 주소창에 `cmd`를 입력하거나 터미널을 열고 다음 명령어를 실행합니다.
  ```cmd
  gemini_parser_win.exe
  ```

---

## 다시 빌드하기

코드 변경 후 직접 다시 빌드하려는 경우 `gemini_parser` 디렉토리 내부에서 아래의 명령어로 빌드합니다.

- **macOS (M1/M2/M3)**: `GOOS=darwin GOARCH=arm64 go build -o ../build/gemini_parser_mac_arm64 main.go`
- **macOS (Intel)**: `GOOS=darwin GOARCH=amd64 go build -o ../build/gemini_parser_mac_amd64 main.go`
- **Windows**: `GOOS=windows GOARCH=amd64 go build -o ../build/gemini_parser_win.exe main.go`
- **Linux**: `GOOS=linux GOARCH=amd64 go build -o ../build/gemini_parser_linux main.go`

---

## 결과물

파싱이 완료되면 아래와 같이 URL별 각 `[순번]_[정제된제목].csv` 파일이 생성됩니다.

### 생성된 CSV 파일 구조

| [대화 메타데이터] |                                       |
| :---------------- | :------------------------------------ |
| 대화 제목         | 테스트 재미나이 대화제목              |
| 공유 URL          | https://share.gemini.google/test_url1 |
| 마지막 사용 모델  | 3.1 Pro                               |
| 대화 시작일       | 2026년 7월 11일 오전 01:37            |
| 링크 게시일       | 2026년 7월 11일 오전 01:52에 게시됨   |
|                   |                                       |

| No. | 질문  | 대답                 |
| :-- | :---- | :------------------- |
| 1   | 질문1 | 대답1... (답변 내용) |
| 2   | 질문2 | 대답2... (답변 내용) |
