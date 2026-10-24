#!/bin/bash
# todo.sh - godic/src 아래 Go 소스의 TODO(TAG) 주석을 태그별로 집계하여 대시보드로 출력한다.
#
# Usage:
#   ./todo.sh              태그별 요약 테이블
#   ./todo.sh -v           전체 상세 목록
#   ./todo.sh -v TAG       특정 태그만 상세 목록
#
# TODO 주석 컨벤션:
#   // TODO(TAG): 설명
#   TAG는 대문자+밑줄 (예: RDB, EVENT_LOOP, SENTINEL)

# ${0%/*} : 이 스크립트의 디렉터리 (Parameter Expansion - Remove Suffix Pattern)
SRC="${0%/*}/src"

# -r : recursive, 하위 디렉터리까지 검색
# -h : 파일명 생략 (매칭된 내용만 출력)
# --include="*.go" : .go 파일만 대상
# grep -o : 매칭된 부분만 출력 (줄 전체가 아닌 패턴 부분만)
# sed 's/TODO(\([A-Z_]*\))/\1/' : TODO(TAG)에서 TAG만 추출 (\1 = 첫 번째 캡처 그룹)
# sort -u : 중복 제거 후 알파벳순 정렬 (-u = --unique)
TAGS=$(grep -rh "TODO([A-Z_]*)" "$SRC" --include="*.go" | grep -o 'TODO([A-Z_]*)' | sed 's/TODO(\([A-Z_]*\))/\1/' | sort -u)
TOTAL=0

# \033[1;36m : bold + cyan (ANSI escape), \033[0m : reset
printf "\n\033[1;36m%-16s %5s  %s\033[0m\n" "TAG" "COUNT" "FILES"
printf "%-16s %5s  %s\n" "────────────────" "─────" "──────────────────────────────"

for tag in $TAGS; do
    # -r : recursive
    # -n : 줄 번호 출력
    hits=$(grep -rn "TODO($tag)" "$SRC" --include="*.go")

    # wc -l : 줄 수 카운트 (= 매칭 횟수)
    count=$(echo "$hits" | wc -l)
    TOTAL=$((TOTAL + count))

    # sed "s|$SRC/||" : 경로에서 $SRC/ 접두사 제거 (구분자 | 사용, 경로에 / 포함되므로)
    # cut -d: -f1 : : 구분자 기준 첫 번째 필드 (파일명)
    # sort -u : 중복 파일명 제거
    # paste -sd, - : 여러 줄을 쉼표로 합침 (-s = serial, -d, = delimiter는 콤마)
    files=$(echo "$hits" | sed "s|$SRC/||" | cut -d: -f1 | sort -u | paste -sd, -)
    printf "%-16s %5d  %s\n" "$tag" "$count" "$files"
done

printf "%-16s %5s\n" "────────────────" "─────"
# \033[1m : bold
printf "\033[1m%-16s %5d\033[0m\n\n" "TOTAL" "$TOTAL"

# -v 옵션: 개별 TODO 항목을 파일:줄번호와 함께 출력
if [ "$1" = "-v" ]; then
    # $2가 있으면 해당 태그만 필터링
    TAG_FILTER="$2"
    # \033[1;33m : bold + yellow
    printf "\033[1;33m── Details"
    # -n : 문자열이 비어있지 않으면 참
    [ -n "$TAG_FILTER" ] && printf " [%s]" "$TAG_FILTER"
    printf " ──\033[0m\n\n"

    # ${TAG_FILTER:-[A-Z_]*} : TAG_FILTER가 비어있으면 모든 태그 매칭 (Parameter Expansion - Default Value)
    pattern="TODO(${TAG_FILTER:-[A-Z_]*})"

    # IFS=: read -r file line content : grep 출력(파일:줄번호:내용)을 : 기준으로 분리
    #   -r : 백슬래시 이스케이프 비활성화 (raw)
    #   content에 나머지 전부 들어감 (3번째 이후 필드)
    grep -rn "$pattern" "$SRC" --include="*.go" | sed "s|$SRC/||" | while IFS=: read -r file line content; do
        # head -1 : TODO가 한 줄에 여러 번 나올 경우 첫 번째만
        tag=$(echo "$content" | grep -o 'TODO([A-Z_]*)' | head -1)

        # 첫 번째 sed: TODO(TAG) 이후의 설명 텍스트만 추출 ([: ]* = 콜론이나 공백 제거)
        # 두 번째 sed: 줄 끝의 주석 닫기 문자(/, *, 공백) 제거
        # xargs : 앞뒤 공백 trim
        desc=$(echo "$content" | sed 's/.*TODO([A-Z_]*)[: ]*//' | sed 's|[/ *]*$||' | xargs)

        # \033[33m : yellow, \033[2m : dim
        printf "  \033[33m%-14s\033[0m \033[2m%-28s:%s\033[0m  %s\n" "$tag" "$file" "$line" "$desc"
    done
    echo
fi
