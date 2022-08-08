package index

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/zinclabs/zinc/pkg/core"
	"github.com/zinclabs/zinc/pkg/meta"
	"github.com/zinclabs/zinc/pkg/zutils"
)

type Alias struct {
	Actions []Action `json:"actions"`
}

type Action struct {
	Add    *base `json:"add"`
	Remove *base `json:"remove"`
}

type base struct {
	Index   string   `json:"index"`
	Alias   string   `json:"alias"`
	Indices []string `json:"indices"`
	Aliases []string `json:"aliases"`
}

func AddOrRemoveESAlias(c *gin.Context) {
	var alias Alias
	err := zutils.GinBindJSON(c, &alias)
	if err != nil {
		c.JSON(http.StatusBadRequest, meta.HTTPResponseError{Error: err.Error()})
		return
	}

	addMap := map[string][]string{}
	removeMap := map[string][]string{}

	indexList := core.ZINC_INDEX_LIST.List()

outerLoop:
	for _, action := range alias.Actions {
		if action.Add != nil {
			if action.Add.Index != "" {
				matchAndAddToMap(indexList, action.Add.Index, addMap, action.Add)
				continue outerLoop
			}

			// index is empty, try the indices field
			for _, indexName := range action.Add.Indices {
				matchAndAddToMap(indexList, indexName, addMap, action.Add)
			}

			continue outerLoop // this was an add action, don't bother checking action.Remove
		}

		if action.Remove != nil {
			if action.Remove.Index != "" {
				matchAndAddToMap(indexList, action.Remove.Index, removeMap, action.Remove)
				continue outerLoop
			}

			// index is empty, try the indices field
			for _, indexName := range action.Remove.Indices {
				matchAndAddToMap(indexList, indexName, removeMap, action.Remove)
			}
		}
	}

	var aliases []string
	for _, index := range indexList {
		aliases = addMap[index.GetName()]
		if aliases != nil {
			index.AddAliases(aliases)
		}

		aliases = removeMap[index.GetName()]
		if aliases != nil {
			index.RemoveAliases(aliases)
		}
	}

	c.JSON(http.StatusOK, gin.H{"acknowledged": true})
}

type M map[string]interface{}

func GetESAliases(c *gin.Context) {
	targetIndex := c.Param("target")
	indexList, ok := getIndexList(targetIndex)
	if !ok {
		c.JSON(http.StatusBadRequest, meta.HTTPResponseError{Error: "index not found"})
		return
	}

	targetAlias := c.Param("target_alias")

	var targetAliases []string
	if targetAlias != "" {
		targetAliases = strings.Split(targetAlias, ",")
	}

	aliases := M{}
	for _, index := range indexList {
		als := M{}

		for _, alias := range index.GetAliases() {
			if targetAlias != "" && !zutils.SliceExists(targetAliases, alias) { // check if this is the alias we're looking for
				continue
			}

			als[alias] = struct{}{}
		}

		if len(als) > 0 {
			aliases[index.GetName()] = M{
				"aliases": als,
			}
		}
	}

	c.JSON(http.StatusOK, aliases)
}

func getIndexList(target string) ([]*core.Index, bool) {
	if target != "" {
		targets := strings.Split(target, ",")
		indexList := make([]*core.Index, 0, len(targets))
		for _, t := range targets {
			index, ok := core.ZINC_INDEX_LIST.Get(t)
			if !ok {
				return nil, false
			}
			indexList = append(indexList, index)
		}
		return indexList, true
	}
	return core.ZINC_INDEX_LIST.List(), true
}

func indexNameMatches(name, indexName string) bool {
	if name == indexName {
		return true
	}

	if strings.Contains(name, "*") {
		p, err := getRegex(name)
		if err != nil {
			log.Err(err).Msg("failed to compile regex")
			return false
		}

		return p.MatchString(indexName)
	}

	return false
}

func getRegex(s string) (*regexp.Regexp, error) {
	parts := strings.Split(s, "*")
	pattern := ""
	for i, part := range parts {
		pattern += part
		if i < len(parts)-1 {
			pattern += "[a-zA-Z0-9_.-]+"
		}
	}

	p, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	return p, nil
}

func matchAndAddToMap(indexList []*core.Index, indexName string, m map[string][]string, b *base) {
	for _, index := range indexList {
		n := index.GetName()
		if indexNameMatches(indexName, n) {
			if b.Alias != "" { // alias takes precedence over aliases
				m[n] = append(m[n], b.Alias)
			} else {
				m[n] = append(m[n], b.Aliases...)
			}
		}
	}
}
