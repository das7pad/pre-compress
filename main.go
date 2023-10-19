/*
 * Pre-Compress
 * Copyright (C) 2023 Jakob Ackermann <das7pad@outlook.com>
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published
 * by the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/das7pad/pre-compress/pkg/pre-compress"
)

func main() {
	rawMTime := flag.String("m-time", time.Time{}.Format(time.RFC3339), "m-time for all files")
	concurrency := flag.Int("concurrency", runtime.NumCPU(), "concurrency")
	flag.Parse()

	mTime, err := time.Parse(time.RFC3339, *rawMTime)
	if err != nil {
		panic(fmt.Errorf("invalid m-time: %w", err))
	}
	root, err := os.Getwd()
	if err != nil {
		panic(fmt.Errorf("get cwd: %w", err))
	}

	n, err := precompress.Recursive(root, mTime, *concurrency, flag.Args())
	fmt.Printf("%d pre-compressed\n", n)

	if err != nil {
		panic(fmt.Errorf("failed: %w", err))
	}
}
