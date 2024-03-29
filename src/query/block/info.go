// Copyright (c) 2019 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package block

func (t BlockType) String() string {
	switch t {
	case BlockM3TSZCompressed:
		return "compressed_m3tsz"
	case BlockDecompressed:
		return "decompressed"
	case BlockScalar:
		return "scalar"
	case BlockLazy:
		return "lazy"
	case BlockTime:
		return "time"
	case BlockContainer:
		return "container"
	case BlockEmpty:
		return "empty"
	case BlockMultiSeries:
		return "multiseries"
	case BlockConsolidated:
		return "consolidated"
	}

	return "unknown"
}

type BlockInfo struct {
	blockType BlockType
	inner     []BlockType
}

func NewBlockInfo(blockType BlockType) BlockInfo {
	return BlockInfo{blockType: blockType}
}

func NewWrappedBlockInfo(
	blockType BlockType,
	wrap BlockInfo,
) BlockInfo {
	inner := make([]BlockType, len(wrap.inner)+1)
	copy(inner[:1], wrap.inner)
	inner[0] = wrap.blockType
	return BlockInfo{
		blockType: blockType,
		inner:     inner,
	}
}

func (b BlockInfo) Type() BlockType {
	return b.blockType
}

func (b BlockInfo) InnerType() BlockType {
	if b.inner == nil {
		return b.Type()
	}

	return b.inner[0]
}

func (b BlockInfo) BaseType() BlockType {
	if b.inner == nil {
		return b.Type()
	}

	return b.inner[len(b.inner)-1]
}
