#!/usr/bin/env python3
import argparse
import json
import mimetypes
import os
import re
import subprocess
import time
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

import requests


class StrapiSeeder:
    def __init__(
        self,
        base_url: str,
        token: str,
        dry_run: bool = False,
        timeout: int = 60,
        upload_timeout: int = 900,
        max_retries: int = 3,
        retry_delay_seconds: float = 2.0,
    ):
        self.base_url = base_url.rstrip("/")
        self.api_url = f"{self.base_url}/api"
        self.token = token
        self.dry_run = dry_run
        self.timeout = timeout
        self.upload_timeout = upload_timeout
        self.max_retries = max_retries
        self.retry_delay_seconds = retry_delay_seconds
        self.headers = {
            "Authorization": f"Bearer {self.token}",
            "Content-Type": "application/json",
        }

    def _request(self, method: str, endpoint: str, **kwargs):
        url = f"{self.api_url}/{endpoint.lstrip('/')}"
        kwargs.setdefault("timeout", self.timeout)

        if self.dry_run and method.upper() in {"POST", "PUT", "PATCH", "DELETE"}:
            payload = kwargs.get("json")
            print(f"[DRY-RUN] {method.upper()} {url}")
            if payload is not None:
                print(json.dumps(payload, indent=2, ensure_ascii=False))
            return None

        last_error: Optional[Exception] = None
        for attempt in range(1, self.max_retries + 1):
            try:
                res = requests.request(method=method, url=url, headers=self.headers, **kwargs)
                if not res.ok:
                    raise RuntimeError(f"{method.upper()} {url} failed: {res.status_code} {res.text}")
                return res.json()
            except Exception as exc:  # noqa: BLE001
                last_error = exc
                if attempt >= self.max_retries:
                    break
                wait_for = self.retry_delay_seconds * attempt
                print(f"[WARN] {method.upper()} {url} failed on attempt {attempt}/{self.max_retries}: {exc}")
                print(f"[WARN] Retrying in {wait_for:.1f}s...")
                time.sleep(wait_for)

        raise RuntimeError(f"{method.upper()} {url} failed after {self.max_retries} attempts: {last_error}")

    def create_entry(self, endpoint: str, data: Dict[str, Any]) -> Optional[str]:
        payload = {"data": data}
        response = self._request("POST", endpoint, json=payload)
        if self.dry_run:
            return f"dryrun-{endpoint}-id"
        return response["data"]["documentId"]

    def update_entry(self, endpoint: str, document_id: str, data: Dict[str, Any]) -> Optional[str]:
        payload = {"data": data}
        response = self._request("PUT", f"{endpoint}/{document_id}", json=payload)
        if self.dry_run:
            return document_id
        return response["data"]["documentId"]

    def upload_media(self, file_path: str, upload_path: str) -> Optional[int]:
        file_name = Path(file_path).name
        mime_type = mimetypes.guess_type(file_name)[0] or "application/octet-stream"

        if self.dry_run:
            print(f"[DRY-RUN] POST {self.api_url}/upload :: file={file_name} path={upload_path}")
            return 1

        upload_url = f"{self.api_url}/upload"
        res = None
        last_error: Optional[Exception] = None

        for attempt in range(1, self.max_retries + 1):
            try:
                with open(file_path, "rb") as file_obj:
                    res = requests.post(
                        upload_url,
                        headers={"Authorization": f"Bearer {self.token}"},
                        files={"files": (file_name, file_obj, mime_type)},
                        data={"path": upload_path},
                        timeout=self.upload_timeout,
                    )

                if not res.ok:
                    raise RuntimeError(f"POST {upload_url} failed: {res.status_code} {res.text}")

                break
            except Exception as exc:  # noqa: BLE001
                last_error = exc
                if attempt >= self.max_retries:
                    break
                wait_for = self.retry_delay_seconds * attempt
                print(f"[WARN] Upload failed for {file_name} on attempt {attempt}/{self.max_retries}: {exc}")
                print(f"[WARN] Retrying upload in {wait_for:.1f}s...")
                time.sleep(wait_for)

        if res is None or not res.ok:
            raise RuntimeError(f"POST {upload_url} failed after {self.max_retries} attempts: {last_error}")

        payload = res.json()
        if isinstance(payload, list) and payload:
            return int(payload[0]["id"])

        if isinstance(payload, dict):
            data = payload.get("data")
            if isinstance(data, list) and data:
                return int(data[0]["id"])
            if isinstance(data, dict) and "id" in data:
                return int(data["id"])

        raise RuntimeError("Unexpected upload response payload from Strapi")

    def find_course_by_title(self, title: str) -> Optional[str]:
        response = self._request("GET", "courses", params={"filters[title][$eq]": title})
        data = response.get("data", [])
        if not data:
            response = self._request("GET", "courses", params={"pagination[pageSize]": 200})
            data = response.get("data", [])
            target = normalize_key(title)
            for item in data:
                candidate = item.get("title", "")
                if normalize_key(candidate) == target:
                    return item.get("documentId")
            for item in data:
                candidate = item.get("title", "")
                normalized_candidate = normalize_key(candidate)
                if target and (target in normalized_candidate or normalized_candidate in target):
                    return item.get("documentId")
            return None
        return data[0].get("documentId")

    def find_training_course_by_text(self, text: str) -> Optional[str]:
        response = self._request("GET", "training-courses", params={"filters[text][$eq]": text})
        data = response.get("data", [])
        if not data:
            response = self._request("GET", "training-courses", params={"pagination[pageSize]": 200})
            data = response.get("data", [])
            target = normalize_key(text)
            for item in data:
                candidate = item.get("text", "")
                if normalize_key(candidate) == target:
                    return item.get("documentId")
            for item in data:
                candidate = item.get("text", "")
                normalized_candidate = normalize_key(candidate)
                if target and (target in normalized_candidate or normalized_candidate in target):
                    return item.get("documentId")
            return None
        return data[0].get("documentId")

    def find_module_by_title(
        self,
        title: str,
        module_type: Optional[str] = None,
        course_document_id: Optional[str] = None,
    ) -> Optional[str]:
        params = {"filters[title][$eq]": title}
        if module_type:
            params["filters[type][$eq]"] = module_type
        if course_document_id:
            params["filters[courses][documentId][$eq]"] = course_document_id

        response = self._request("GET", "course-modules", params=params)
        data = response.get("data", [])
        if not data:
            all_modules = self.list_modules(module_type=module_type, course_document_id=course_document_id)
            target = normalize_key(title)
            for item in all_modules:
                candidate = item.get("title", "")
                if normalize_key(candidate) == target:
                    return item.get("documentId")
            for item in all_modules:
                candidate = item.get("title", "")
                normalized_candidate = normalize_key(candidate)
                if target and (target in normalized_candidate or normalized_candidate in target):
                    return item.get("documentId")
            return None
        return data[0].get("documentId")

    def list_modules(
        self,
        module_type: Optional[str] = None,
        course_document_id: Optional[str] = None,
        page_size: int = 200,
    ) -> List[Dict[str, Any]]:
        page = 1
        all_rows: List[Dict[str, Any]] = []

        while True:
            params = {
                "pagination[page]": page,
                "pagination[pageSize]": page_size,
            }
            if module_type:
                params["filters[type][$eq]"] = module_type
            if course_document_id:
                params["filters[courses][documentId][$eq]"] = course_document_id

            response = self._request("GET", "course-modules", params=params)
            data = response.get("data", [])
            if not data:
                break

            for row in data:
                all_rows.append(
                    {
                        "documentId": row.get("documentId"),
                        "title": row.get("title"),
                        "type": row.get("type"),
                    }
                )

            meta = response.get("meta", {})
            pagination = meta.get("pagination", {})
            page_count = pagination.get("pageCount", page)
            if page >= page_count:
                break
            page += 1

        return all_rows


def load_env_file(env_path: Optional[str]) -> Dict[str, str]:
    values: Dict[str, str] = {}
    if not env_path:
        return values

    path = Path(env_path)
    if not path.exists():
        raise FileNotFoundError(f"Env file not found: {env_path}")

    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip()

    return values


def resolve_config(args: argparse.Namespace) -> Tuple[str, str]:
    env_values = load_env_file(args.env_file)

    base_url = args.base_url or os.getenv("STRAPI_BASE_URL") or env_values.get("STRAPI_BASE_URL")
    token = args.token or os.getenv("STRAPI_API_TOKEN") or env_values.get("STRAPI_API_TOKEN")

    if not base_url:
        raise RuntimeError("STRAPI base URL is missing. Set STRAPI_BASE_URL or pass --base-url.")
    if not token:
        raise RuntimeError("STRAPI API token is missing. Set STRAPI_API_TOKEN or pass --token.")

    return base_url, token


def _normalize_whitespace(text: str) -> str:
    text = text.replace("\u00a0", " ")
    text = text.replace("\u2028", " ")
    text = text.replace("\r", "")
    text = re.sub(r"\s+", " ", text)
    return text.strip()


def normalize_key(text: str) -> str:
    compact = _normalize_whitespace(text).lower()
    compact = re.sub(r"[^a-z0-9]+", "", compact)
    return compact


def convert_docx_to_text(docx_path: str) -> str:
    command = ["textutil", "-convert", "txt", "-stdout", docx_path]
    proc = subprocess.run(command, capture_output=True, text=True, check=False)
    if proc.returncode != 0:
        raise RuntimeError(f"Failed to parse DOCX: {docx_path}\n{proc.stderr}")
    return proc.stdout


def parse_multiple_choice_options(question_raw: str) -> List[Tuple[str, str]]:
    compact = _normalize_whitespace(question_raw)
    matches = re.findall(r"([A-D])\.\s*(.*?)(?=(?:\s+[A-D]\.\s)|$)", compact)
    options: List[Tuple[str, str]] = []
    for label, text in matches:
        cleaned = _normalize_whitespace(text)
        if cleaned:
            options.append((label, cleaned))
    return options


def parse_question_block(body: str) -> Optional[Dict[str, Any]]:
    if "Correct Answer:" not in body:
        return None

    answer_split = re.split(r"Correct\s*Answer\s*:\s*", body, maxsplit=1, flags=re.IGNORECASE)
    if len(answer_split) != 2:
        return None

    question_part = answer_split[0].strip()
    rest = answer_split[1].strip()

    explanation = ""
    correct_answer = rest
    if re.search(r"\bExplanation\s*:\s*", rest, flags=re.IGNORECASE):
        answer_and_expl = re.split(r"Explanation\s*:\s*", rest, maxsplit=1, flags=re.IGNORECASE)
        correct_answer = answer_and_expl[0].strip()
        explanation = answer_and_expl[1].strip() if len(answer_and_expl) > 1 else ""

    # Some DOCX exports inline the next section heading into the current explanation.
    explanation = re.sub(r"\s+\d{2}\.\d{2}\s+.+$", "", explanation)

    type_match = re.match(
        r"(?is)\s*(True\s*or\s*False|Multiple\s*Choice|Fill\s*in\s*the\s*Blank)\s*:\s*(.*)",
        question_part,
    )

    if type_match:
        q_label = _normalize_whitespace(type_match.group(1)).lower()
        prompt = _normalize_whitespace(type_match.group(2))
    else:
        q_label = "multiple choice"
        prompt = _normalize_whitespace(question_part)

    answer_value = _normalize_whitespace(correct_answer).strip(". ")

    question_type = "single-choice"
    answer_options: List[Dict[str, Any]] = []

    if "true or false" in q_label:
        question_type = "true-false"
        normalized = answer_value.lower()
        is_true = normalized in {"true", "t", "yes"}
        answer_options = [
            {"text": "True", "is_correct": is_true},
            {"text": "False", "is_correct": not is_true},
        ]
    elif "fill in the blank" in q_label:
        question_type = "single-choice"
        answer_options = [{"text": answer_value, "is_correct": True}]
    else:
        options = parse_multiple_choice_options(question_part)
        if options:
            letter_answer = answer_value.upper().strip()
            for label, text in options:
                is_correct = letter_answer == label.upper() or answer_value.lower() == text.lower()
                answer_options.append({"text": text, "is_correct": is_correct})

            if not any(opt["is_correct"] for opt in answer_options):
                for opt in answer_options:
                    if opt["text"].lower() == answer_value.lower():
                        opt["is_correct"] = True
                        break
        else:
            answer_options = [{"text": answer_value, "is_correct": True}]

        question_type = "single-choice"

    if not answer_options:
        answer_options = [{"text": answer_value, "is_correct": True}]

    return {
        "prompt": prompt,
        "question_type": question_type,
        "explanation": _normalize_whitespace(explanation),
        "answer_options": answer_options,
    }


def parse_quiz_docx(docx_path: str) -> Dict[str, Any]:
    raw_text = convert_docx_to_text(docx_path)

    block_pattern = re.compile(r"(?ms)^\s*(\d+)\.\s+(.*?)(?=^\s*\d+\.\s+|\Z)")
    blocks = list(block_pattern.finditer(raw_text))
    questions: List[Dict[str, Any]] = []

    for block in blocks:
        body = block.group(2).strip()
        parsed = parse_question_block(body)
        if parsed:
            questions.append(parsed)

    if not questions:
        raise RuntimeError(f"No quiz questions parsed from DOCX: {docx_path}")

    return {
        "title": Path(docx_path).stem,
        "format": "single-choice",
        "quiz_type": "scored",
        "questions": questions,
    }


def parse_quiz_docx_by_sections(docx_path: str) -> Dict[str, Dict[str, Any]]:
    raw_text = convert_docx_to_text(docx_path)

    heading_pattern = re.compile(r"(?m)^\s*(\d{2}\.\d{2})\s+(.+?)\s*$")
    headings = list(heading_pattern.finditer(raw_text))

    block_pattern = re.compile(r"(?ms)^\s*(\d+)\.\s+(.*?)(?=^\s*\d+\.\s+|\Z)")
    blocks = list(block_pattern.finditer(raw_text))

    if not headings:
        return {"default": parse_quiz_docx(docx_path)}

    section_titles: Dict[str, str] = {}
    for match in headings:
        section_titles[match.group(1)] = _normalize_whitespace(match.group(2))

    section_questions: Dict[str, List[Dict[str, Any]]] = {k: [] for k in section_titles.keys()}

    for block in blocks:
        block_start = block.start()
        section_key = None
        for heading in headings:
            if heading.start() <= block_start:
                section_key = heading.group(1)
            else:
                break

        if section_key is None:
            continue

        parsed = parse_question_block(block.group(2).strip())
        if parsed:
            section_questions[section_key].append(parsed)

    result: Dict[str, Dict[str, Any]] = {}
    base_title = Path(docx_path).stem
    for key, questions in section_questions.items():
        if not questions:
            continue
        section_title = section_titles.get(key, key)
        result[key] = {
            "title": f"{section_title} Quiz",
            "format": "single-choice",
            "quiz_type": "scored",
            "questions": questions,
            "_source": base_title,
        }

    if not result:
        result["default"] = parse_quiz_docx(docx_path)

    return result


def seed_module_quiz(
    client: StrapiSeeder,
    module_id: str,
    quiz_payload: Dict[str, Any],
    defaults: Dict[str, Any],
) -> Dict[str, Any]:
    quiz_data = {
        "title": quiz_payload.get("title") or "Module Quiz",
        "quiz_type": quiz_payload.get("quiz_type", defaults.get("quiz_type", "scored")),
        "format": quiz_payload.get("format", defaults.get("format", "single-choice")),
        "passing_score": quiz_payload.get("passing_score", defaults.get("passing_score", 80)),
        "estimated_completion_time_in_mins": quiz_payload.get(
            "estimated_completion_time_in_mins",
            defaults.get("estimated_completion_time_in_mins", 20),
        ),
    }

    quiz_id = client.create_entry("quizzes", quiz_data)
    client.update_entry("course-modules", module_id, {"quiz": quiz_id})

    question_ids = []
    created_questions = 0
    created_options = 0

    for question in quiz_payload["questions"]:
        q_id = client.create_entry(
            "questions",
            {
                "prompt": question["prompt"],
                "question_type": question["question_type"],
                "explanation": question.get("explanation", ""),
                "quiz": quiz_id,
            },
        )
        question_ids.append(q_id)
        created_questions += 1

        option_ids = []
        for option in question.get("answer_options", []):
            option_id = client.create_entry(
                "answer-options",
                {
                    "text": option["text"],
                    "is_correct": bool(option["is_correct"]),
                    "questions": [q_id],
                },
            )
            option_ids.append(option_id)
            created_options += 1

        if option_ids:
            client.update_entry("questions", q_id, {"answer_options": option_ids})

    client.update_entry("quizzes", quiz_id, {"questions": question_ids})

    return {
        "module_id": module_id,
        "quiz_id": quiz_id,
        "question_count": created_questions,
        "option_count": created_options,
    }


def load_manifest(path: str) -> Dict[str, Any]:
    manifest_path = Path(path)
    if not manifest_path.exists():
        raise FileNotFoundError(f"Manifest not found: {path}")
    return json.loads(manifest_path.read_text(encoding="utf-8"))


def resolve_module_id(
    client: StrapiSeeder,
    module_entry: Dict[str, Any],
    mode: str,
    resolve_by_title: bool,
    course_document_id: Optional[str],
) -> str:
    module_id = module_entry.get("module_document_id") or module_entry.get("document_id") or module_entry.get("id")

    if module_id:
        return module_id

    if resolve_by_title:
        title = module_entry.get("title")
        if not title:
            raise RuntimeError("Module title is required for title-based resolution")
        resolved = client.find_module_by_title(
            title=title,
            module_type=module_entry.get("type", "course"),
            course_document_id=course_document_id,
        )
        if resolved:
            return resolved

    if mode == "quizzes-only":
        raise RuntimeError(
            "Missing module ID for quizzes-only mode. Provide module_document_id or use --resolve-modules-by-title."
        )

    raise RuntimeError("Full mode is not implemented in this seeder version. Use quizzes-only mode.")


def split_video_name(video_file: Path) -> Tuple[Optional[str], str]:
    stem = video_file.stem
    match = re.match(r"^\s*(\d{2}\.\d{2})\s*(.+)$", stem)
    if not match:
        cleaned = stem.replace("_", "'")
        return None, _normalize_whitespace(cleaned)

    section_key = match.group(1)
    title = match.group(2).replace("_", "'")
    return section_key, _normalize_whitespace(title)


def seed_social_course_from_assets(
    client: StrapiSeeder,
    assets_root: str,
    training_course_text: str,
    training_course_description: str,
    reuse_training_course_if_exists: bool,
    reuse_course_if_exists: bool,
    defaults: Dict[str, Any],
    upload_path: str,
) -> List[Dict[str, Any]]:
    assets_path = Path(assets_root)
    if not assets_path.exists() or not assets_path.is_dir():
        raise RuntimeError(f"Assets folder not found: {assets_root}")

    training_course_id = None
    if reuse_training_course_if_exists:
        training_course_id = client.find_training_course_by_text(training_course_text)

    if not training_course_id:
        training_course_id = client.create_entry(
            "training-courses",
            {
                "text": training_course_text,
                "description": training_course_description,
            },
        )

    chapter_course_ids: List[str] = []
    summary: List[Dict[str, Any]] = []
    order_index = 1

    module_folders = [p for p in sorted(assets_path.iterdir()) if p.is_dir()]
    for folder in module_folders:
        chapter_title = _normalize_whitespace(folder.name)
        chapter_course_id = None
        if reuse_course_if_exists:
            chapter_course_id = client.find_course_by_title(chapter_title)

        if not chapter_course_id:
            chapter_course_id = client.create_entry(
                "courses",
                {
                    "title": chapter_title,
                    "description": f"Social media chapter: {chapter_title}",
                },
            )

        chapter_course_ids.append(chapter_course_id)

        docx_candidates = sorted(folder.glob("*Quiz.docx"))
        if not docx_candidates:
            print(f"[WARN] Skipping {folder.name} - quiz DOCX not found")
            continue

        docx_path = str(docx_candidates[0])
        section_quizzes = parse_quiz_docx_by_sections(docx_path)
        default_quiz = parse_quiz_docx(docx_path)

        video_candidates: List[Path] = []
        for ext in ("*.mp4", "*.mov", "*.m4v", "*.webm", "*.avi"):
            video_candidates.extend(sorted(folder.glob(ext)))

        if not video_candidates:
            print(f"[WARN] Skipping {folder.name} - no video files found")
            continue

        chapter_module_ids: List[str] = []

        for video_file in sorted(video_candidates):
            section_key, module_title = split_video_name(video_file)
            quiz_payload = section_quizzes.get(section_key) or section_quizzes.get("default") or default_quiz

            module_id = client.create_entry(
                "course-modules",
                {
                    "title": module_title,
                    "description": f"Auto-seeded from {folder.name}",
                    "type": "course",
                    "courses": [chapter_course_id],
                },
            )
            chapter_module_ids.append(module_id)

            media_id = client.upload_media(str(video_file), upload_path)
            client.update_entry("course-modules", module_id, {"video": media_id})

            seeded = seed_module_quiz(client, module_id, quiz_payload, defaults)
            seeded.update(
                {
                    "training_course_id": training_course_id,
                    "course_title": chapter_title,
                    "course_id": chapter_course_id,
                    "module_title": module_title,
                    "module_id": module_id,
                    "video_file": str(video_file),
                    "quiz_source_docx": docx_path,
                    "section_key": section_key,
                    "order_in_course": order_index,
                }
            )
            summary.append(seeded)
            order_index += 1

        if chapter_module_ids:
            client.update_entry("courses", chapter_course_id, {"course_modules": chapter_module_ids})

    # Strict sequence link: TrainingCourse -> chapter Courses
    if chapter_course_ids:
        client.update_entry("training-courses", training_course_id, {"courses": chapter_course_ids})

    return summary


def run_seed(args: argparse.Namespace):
    base_url, token = resolve_config(args)
    manifest = load_manifest(args.manifest) if args.manifest else {}

    defaults = manifest.get("defaults", {"quiz_type": "scored", "format": "single-choice", "passing_score": 80, "estimated_completion_time_in_mins": 20})
    courses = manifest.get("courses", [])

    client = StrapiSeeder(
        base_url=base_url,
        token=token,
        dry_run=args.dry_run,
        timeout=args.timeout,
        upload_timeout=args.upload_timeout,
        max_retries=args.max_retries,
        retry_delay_seconds=args.retry_delay_seconds,
    )

    if args.list_modules:
        rows = client.list_modules(module_type=args.module_type_filter, course_document_id=args.course_document_id_filter)
        print(json.dumps(rows, indent=2, ensure_ascii=False))
        return

    if args.seed_social_from_assets:
        summary = seed_social_course_from_assets(
            client=client,
            assets_root=args.seed_social_from_assets,
            training_course_text=args.training_course_text,
            training_course_description=args.training_course_description,
            reuse_training_course_if_exists=args.reuse_training_course_if_exists,
            reuse_course_if_exists=args.reuse_course_if_exists,
            defaults=defaults,
            upload_path=args.upload_path,
        )
        print("\n=== Seeder Summary ===")
        print(json.dumps(summary, indent=2, ensure_ascii=False))
        return

    if not courses:
        raise RuntimeError("Manifest has no courses. Provide --manifest or use --seed-social-from-assets.")

    summary = []

    for course in courses:
        course_key = course.get("course_key", "unknown")
        course_document_id = course.get("course_document_id") or course.get("document_id")
        modules = course.get("modules", [])

        if not modules:
            print(f"[INFO] Skipping course '{course_key}' - no modules in manifest")
            continue

        for module in modules:
            module_id = resolve_module_id(
                client=client,
                module_entry=module,
                mode=args.mode,
                resolve_by_title=args.resolve_modules_by_title,
                course_document_id=course_document_id,
            )

            quiz_config = module.get("quiz", {})
            docx_path = module.get("quiz_docx_path")

            if docx_path:
                quiz_payload = parse_quiz_docx(docx_path)
                quiz_payload.update({
                    "title": quiz_config.get("title", quiz_payload.get("title")),
                    "quiz_type": quiz_config.get("quiz_type", quiz_payload.get("quiz_type")),
                    "format": quiz_config.get("format", quiz_payload.get("format")),
                    "passing_score": quiz_config.get("passing_score", defaults.get("passing_score", 80)),
                    "estimated_completion_time_in_mins": quiz_config.get(
                        "estimated_completion_time_in_mins",
                        defaults.get("estimated_completion_time_in_mins", 20),
                    ),
                })
            else:
                if "questions" not in quiz_config:
                    raise RuntimeError(
                        f"Module '{module.get('title', module_id)}' has no quiz_docx_path and no inline quiz questions."
                    )
                quiz_payload = quiz_config

            result = seed_module_quiz(client, module_id, quiz_payload, defaults)
            result.update(
                {
                    "course_key": course_key,
                    "module_title": module.get("title", ""),
                    "source": docx_path or "inline",
                }
            )
            summary.append(result)
            print(
                f"[OK] {course_key} :: {module.get('title', module_id)} -> quiz {result['quiz_id']} "
                f"questions={result['question_count']} options={result['option_count']}"
            )

    print("\n=== Seeder Summary ===")
    print(json.dumps(summary, indent=2, ensure_ascii=False))


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Seed Strapi quizzes/questions/options for existing modules.")
    parser.add_argument("--manifest", help="Path to manifest JSON file")
    parser.add_argument("--env-file", default="app.development.env", help="Optional env file (KEY=VALUE)")
    parser.add_argument("--base-url", help="Override Strapi base URL")
    parser.add_argument("--token", help="Override Strapi API token")
    parser.add_argument("--mode", choices=["quizzes-only", "full"], default="quizzes-only")
    parser.add_argument("--resolve-modules-by-title", action="store_true")
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--timeout", type=int, default=60)
    parser.add_argument("--upload-timeout", type=int, default=900, help="Timeout in seconds for media upload requests")
    parser.add_argument("--max-retries", type=int, default=3, help="Max retry attempts for network calls")
    parser.add_argument("--retry-delay-seconds", type=float, default=2.0, help="Base delay between retries")
    parser.add_argument("--list-modules", action="store_true", help="List existing Strapi modules and exit")
    parser.add_argument("--module-type-filter", default=None, help="Optional module type filter for --list-modules")
    parser.add_argument("--course-document-id-filter", default=None, help="Optional course document ID filter for --list-modules")
    parser.add_argument("--seed-social-from-assets", default=None, help="Seed full social course from local assets folder")
    parser.add_argument("--training-course-text", default="Social Media Driver's Training Course", help="TrainingCourse text value")
    parser.add_argument("--training-course-description", default="Structured social media training course", help="TrainingCourse description value")
    parser.add_argument("--social-course-title", default="Social Media Driver's Training Course", help="Course title for social media seeding")
    parser.add_argument("--social-course-description", default="Auto-seeded social media course content", help="Course description for social media seeding")
    parser.add_argument("--reuse-training-course-if-exists", action="store_true", help="Reuse existing TrainingCourse with same text instead of creating a new one")
    parser.add_argument("--reuse-course-if-exists", action="store_true", help="Reuse existing course with same title instead of creating a new one")
    parser.add_argument("--upload-path", default="app/social_media_course_videos", help="Strapi upload folder path")
    return parser.parse_args()


if __name__ == "__main__":
    run_seed(parse_args())
